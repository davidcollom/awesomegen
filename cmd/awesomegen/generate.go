package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"

	"github.com/davidcollom/awesomegen/internal/config"
	httpgh "github.com/davidcollom/awesomegen/internal/github"
	"github.com/davidcollom/awesomegen/internal/render"
	"github.com/davidcollom/awesomegen/internal/scrape"
)

// newGenerateCommand builds the command that generates the README files.
//
// The flags are inherited from the root command, so that the plain
// "awesomegen" invocation and "awesomegen generate" behave the same way.
func newGenerateCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate the README(s) from the Star Lists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGenerate(cmd, opts)
		},
	}
}

// generator holds the collaborators of a generation run.
//
// They are fields rather than locals so that a test can replace the scraper
// and the GitHub client with fakes and run the whole generation offline,
// without a network connection.
type generator struct {
	Scraper scrape.Scraper
	GH      httpgh.Client
	Clock   clockwork.Clock
	Log     logr.Logger
}

// newGenerator returns the generator that talks to GitHub.
func newGenerator(log logr.Logger) *generator {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		log.Info("no GitHub token found, the API rate limit will be low",
			"hint", "set GH_TOKEN or GITHUB_TOKEN")
	}
	return &generator{
		Scraper: scrape.NewScraper(log), // HTML scraper for Star Lists
		GH:      httpgh.New(token),      // GitHub REST client (enrichment)
		Clock:   clockwork.NewRealClock(),
		Log:     log,
	}
}

// runGenerate scrapes, enriches, filters and renders every list of the
// configuration file.
func runGenerate(cmd *cobra.Command, opts *options) error {
	return runGenerateWith(cmd, opts, newGenerator(loggerFrom(cmd.Context())))
}

// runGenerateWith runs a generation with the given collaborators.
func runGenerateWith(cmd *cobra.Command, opts *options, gen *generator) error {
	ctx := cmd.Context()
	log := gen.Log

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}

	// Every output path is resolved relative to the directory that holds the
	// configuration file.
	root, err := configRoot(opts.configPath)
	if err != nil {
		return err
	}

	// Resolve every output path up front so two lists cannot silently
	// overwrite each other.
	outputs, err := resolveOutputs(cfg, opts.configPath)
	if err != nil {
		return err
	}

	// Every output path is checked before the first request is sent. The
	// check is the one the write performs, so that a configuration that is
	// reported as valid is one the generator can write. It runs in a dry
	// run as well: a rehearsal that accepts a path the run refuses is
	// worse than no rehearsal at all.
	if err := checkOutputPaths(root, outputs); err != nil {
		return err
	}

	enr := render.Enricher{GH: gen.GH, Now: gen.Clock, Log: log}

	for i, list := range cfg.Lists {
		// 1) scrape owner/repo slugs from your Star List
		var allSlugs []string
		seen := map[string]struct{}{}
		for _, src := range list.Sources {
			log.V(1).Info("scraping star list", "user", src.User, "slug", src.Slug)
			slugs, err := gen.Scraper.ListRepos(ctx, src.User, src.Slug)
			if err != nil {
				return fmt.Errorf("scrape %s/%s: %w", src.User, src.Slug, err)
			}
			log.V(1).Info("scraped star list", "user", src.User, "slug", src.Slug, "repositories", len(slugs))
			for _, r := range slugs {
				if _, ok := seen[r]; ok {
					continue
				}
				seen[r] = struct{}{}
				allSlugs = append(allSlugs, r)
			}
		}
		list.SeedRepos(allSlugs)

		// 2) enrich + filter
		enriched, err := enr.EnrichAndFilter(ctx, list)
		if err != nil {
			var lookupErr *render.LookupError
			if !errors.As(err, &lookupErr) {
				// A lookup that failed for a reason that has nothing to
				// do with the repository - an expired token, a rate
				// limit, a server error, a network problem - leaves the
				// list unknown rather than curated. The README is not
				// written, and the message names the file that is left
				// alone.
				var unavailable *render.UnavailableError
				if errors.As(err, &unavailable) {
					return fmt.Errorf("%w: refusing to overwrite %s with an unknown list", err, outputs[i])
				}
				return err
			}
			// Individual lookup failures are reported but do not abort the
			// run: a single deleted repository should not stop the whole list
			// from being regenerated.
			log.Error(err, "some repository lookups failed", "failed", lookupErr.Failed, "total", lookupErr.Total)
			// Every repository of the list is gone, though. The list is then
			// a list of links alone, and writing it would replace the
			// curated README with what is left of the curation. The run is
			// stopped instead, and the message says how to continue: the
			// repositories have to be removed from the configuration.
			if lookupErr.AllFailed() {
				return fmt.Errorf("every repository of the list could not be loaded (%d of %d): refusing to overwrite %s with an incomplete README; remove the repositories from the configuration to render the rest of the list",
					lookupErr.Failed, lookupErr.Total, outputs[i])
			}
		}

		// 3) render markdown
		md, err := render.Markdown(enriched)
		if err != nil {
			// A failing API lookup or an expired token silently drops every
			// repository, and committing that empty result would wipe out a
			// curated list.
			if errors.Is(err, render.ErrNoEntries) {
				return fmt.Errorf("list %q produced no entries; refusing to overwrite %s with an empty README", list.Slug, outputs[i])
			}
			return err
		}

		out := outputs[i]
		if err := writeOutput(cmd, root, out, md, opts.dryRun); err != nil {
			return err
		}
		if opts.dryRun {
			log.Info("dry run, the README was not written", "file", out, "bytes", len(md))
			continue
		}
		log.Info("wrote README", "file", out, "title", list.Title, "bytes", len(md))
	}
	return nil
}

// writeOutput writes the rendered document to its destination, or prints it
// when the run is a dry run.
//
// In a dry run the document goes to the standard output, so that it can be
// inspected or redirected, and the destination is left untouched. The
// comment above the document names the file it would have been written to.
func writeOutput(cmd *cobra.Command, root, out, md string, dryRun bool) error {
	if dryRun {
		rel, err := filepath.Rel(root, out)
		if err != nil {
			rel = out
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "<!-- awesomegen: %s -->\n", commentText(filepath.ToSlash(rel))); err != nil {
			return err
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), md); err != nil {
			return err
		}
		return nil
	}
	return writeFileAtomic(root, out, []byte(md))
}

// commentText makes a text safe to put into an HTML comment.
//
// A comment must not contain "--" and it ends at the first ">", so a path
// that carries them would end the comment early and put the rest of the
// path into the document. The line breaks are replaced as well, so that the
// comment stays on one line.
func commentText(s string) string {
	return strings.NewReplacer(
		"--", "- -",
		">", "&gt;",
		"\r", " ",
		"\n", " ",
	).Replace(s)
}

// configRoot returns the directory that holds the configuration file. Every
// output path is resolved relative to it, so the result does not depend on
// the working directory.
func configRoot(cfgPath string) (string, error) {
	return filepath.Abs(filepath.Dir(cfgPath))
}

// resolveOutputs determines the output path for every list. Relative paths
// are resolved against the directory holding the config file so the result
// does not depend on the working directory.
func resolveOutputs(cfg config.Config, cfgPath string) ([]string, error) {
	base, err := configRoot(cfgPath)
	if err != nil {
		return nil, err
	}
	policy := outputPolicy{caseInsensitive: caseInsensitiveDir(base)}
	return resolveOutputsIn(cfg, base, policy)
}

// outputPolicy describes how the file system that holds the output files
// compares two paths.
type outputPolicy struct {
	// caseInsensitive is true when the file system does not distinguish
	// names that differ only in case.
	caseInsensitive bool
}

// key returns the comparison key of a path.
//
// Two paths with the same key name the same file.
func (p outputPolicy) key(path string) string {
	if p.caseInsensitive {
		return strings.ToLower(path)
	}
	return path
}

// resolveOutputsIn determines the output path for every list, relative to
// the directory that holds the configuration file.
//
// The paths are compared with the given policy, so that two outputs that
// name the same file are refused on every platform.
func resolveOutputsIn(cfg config.Config, base string, policy outputPolicy) ([]string, error) {
	out := make([]string, len(cfg.Lists))
	seen := map[string]int{}
	for i := range cfg.Lists {
		l := &cfg.Lists[i]
		rel := l.Output
		if rel == "" {
			if len(cfg.Lists) == 1 {
				rel = "README.md"
			} else {
				// More than one list: derive a distinct file name per list
				// instead of letting the last list overwrite the others.
				if l.Slug == "" {
					return nil, fmt.Errorf("lists[%d]: output is required when a config contains more than one list", i)
				}
				rel = l.Slug + ".md"
			}
		}
		// Derived file names are held to the same rules as configured ones:
		// the slug is user supplied and must not be able to escape the
		// config directory or target a hidden path.
		if err := config.ValidateOutputPath(rel); err != nil {
			return nil, fmt.Errorf("lists[%d]: output %q: %w", i, rel, err)
		}
		p := rel
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, rel)
		}
		p = filepath.Clean(p)
		key := policy.key(p)
		if prev, ok := seen[key]; ok {
			return nil, fmt.Errorf("lists[%d] and lists[%d] both write to %s; set a distinct output for each list", prev, i, p)
		}
		seen[key] = i
		out[i] = p
	}
	// A path that is the directory of another path cannot hold both: the
	// generator would write one of them and then fail on the other one,
	// because the file is in the way of the directory it needs.
	//
	// The ancestors of a path are looked at instead of comparing every
	// path with every other one: the number of comparisons then grows with
	// the depth of the paths and not with the square of the number of
	// lists, so a configuration with many lists stays cheap to check.
	for i, p := range out {
		for dir := filepath.Dir(p); ; dir = filepath.Dir(dir) {
			if prev, ok := seen[policy.key(dir)]; ok && prev != i {
				return nil, fmt.Errorf("lists[%d] writes to %s, which is inside %s, the file that lists[%d] writes to; set outputs that do not contain each other", i, out[i], out[prev], prev)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	return out, nil
}

// caseInsensitiveDir reports whether the file system that holds dir
// distinguishes names that differ only in case.
//
// The probe uses a file that exists: on a file system that ignores the
// case, a name that differs from it only in case resolves to the same
// file, and on a file system that does not, the name does not exist.
//
// A directory that cannot be read is reported as case-sensitive, which is
// the safe answer: two outputs that differ only in case are then allowed,
// exactly as they are on a file system that tells them apart.
func caseInsensitiveDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		variant := swapCase(name)
		if variant == name {
			// The name has no letters, so it cannot be told apart from
			// its variant by the case.
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, variant)); err == nil {
			return true
		}
		// The first file that carries letters decides: on a file system
		// that ignores the case, its variant exists as well.
		return false
	}
	return false
}

// swapCase returns name with the case of its letters flipped.
func swapCase(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLower(r):
			return unicode.ToUpper(r)
		case unicode.IsUpper(r):
			return unicode.ToLower(r)
		}
		return r
	}, name)
}

// writeFileAtomic writes to a temporary file in the destination directory and
// renames it into place, so an interrupted or failed run never leaves a
// half-written README behind.
//
// The destination has to stay inside root. A symbolic link in the path would
// otherwise redirect the write - and the temporary file that is renamed over
// it - outside of the directory the configuration lives in.
func writeFileAtomic(root, path string, data []byte) error {
	if err := checkWritableOutput(root, path); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory %s: %w", dir, err)
	}

	// The mode of the file that is replaced is kept: a README that only its
	// owner may read does not become readable by everybody. A file that does
	// not exist yet is written with the mode the umask of the user produces,
	// so that a restrictive umask keeps the new file private.
	mode, replaced := os.FileMode(0), false
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		mode, replaced = fi.Mode().Perm(), true
	}

	tmp, err := createTempFile(dir)
	if err != nil {
		return fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	// The data has to be on the disk before the file is renamed into place:
	// a crash right after the rename would otherwise leave a file that is
	// there but empty.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	// The umask only applies while a file is created, so the mode of a file
	// that is replaced is set explicitly: the file keeps the mode it had.
	if replaced {
		if err := os.Chmod(tmpName, mode); err != nil {
			return fmt.Errorf("chmod %s: %w", tmpName, err)
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	// The name of the file has to be on the disk as well. Not every
	// platform can do that, so a failure is not fatal.
	syncDir(dir)
	return nil
}

// tempFileNames is the number of names that are tried before the creation of
// a temporary file is given up.
//
// The names are random, so a name that is taken is rare.
const tempFileNames = 10

// createTempFile creates a temporary file in dir, whose name tells that it
// belongs to a run of the generator.
//
// The file is created with the mode 0666, so that the umask of the user
// decides the mode of a file that is written for the first time. The
// temporary file of os.CreateTemp always has the mode 0600, and a mode that
// is set afterwards ignores the umask: a checkout with a restrictive umask
// would end up with a README that every user of the machine may read.
//
// The name is unique and the file is created exclusively, so that two runs
// cannot end up with the same file.
func createTempFile(dir string) (*os.File, error) {
	for range tempFileNames {
		name, err := tempFileName()
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, ".awesomegen-"+name+".tmp")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o666)
		if err == nil {
			return f, nil
		}
		// A name that is taken is a leftover of another run, so the next
		// name is tried.
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no free name for a temporary file in %s", dir)
}

// tempFileName returns a random name for a temporary file.
//
// The name only has to be unique in the directory of the output, so that two
// runs cannot pick the same one.
func tempFileName() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// syncDir flushes the directory entry of a renamed file to the disk.
//
// Not every platform supports it - Windows refuses to open a directory - so
// a failure is ignored.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer func() { _ = d.Close() }()
	_ = d.Sync()
}

// checkWritableOutput verifies that the destination of an output path can be
// written.
//
// The lexical check of the path is not enough: a directory of the path can
// be a symbolic link that points outside of the configuration directory, and
// the write would follow it. The destination can also be a directory itself,
// which passes every other check but cannot be replaced by a file.
//
// The offline check runs the same function, so that a configuration that is
// reported as valid is one the generator can write.
func checkWritableOutput(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve %s against %s: %w", path, root, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside of %s", path, root)
	}
	if err := checkPathComponents(root, rel); err != nil {
		return err
	}
	return rejectDirectory(root, path)
}

// rejectDirectory reports an error when the destination exists and is a
// directory.
//
// A directory that carries the name of the README - "README.md" as a
// directory, for example - passes every other check, but the write fails
// when the temporary file is renamed over it.
func rejectDirectory(root, path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing is there yet, so the write creates the file.
			return nil
		}
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a directory; refusing to write the README over it", relativeTo(root, path))
	}
	return nil
}

// checkPathComponents verifies that the path components below root can hold
// the file.
//
// Lexical path validation does not stop a write from following a symlink: a
// directory such as "docs" that points outside of the configuration directory
// would move the generated file out of the allowed root. Every component but
// the last one also has to be a directory, because the file is written into
// it.
func checkPathComponents(root, rel string) error {
	segs := make([]string, 0, 4)
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == "" || seg == "." {
			continue
		}
		segs = append(segs, seg)
	}

	cur := root
	for i, seg := range segs {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				// The component does not exist yet, so it is not a symlink
				// and nothing below it can be one either.
				return nil
			}
			return fmt.Errorf("inspect %s: %w", cur, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link; refusing to write through it", cur)
		}
		if i < len(segs)-1 && !fi.IsDir() {
			return fmt.Errorf("%s is not a directory; refusing to write the file into it", cur)
		}
	}
	return nil
}
