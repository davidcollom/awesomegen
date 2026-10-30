package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// runGenerate scrapes, enriches, filters and renders every list of the
// configuration file.
func runGenerate(cmd *cobra.Command, opts *options) error {
	ctx := cmd.Context()
	log := loggerFrom(ctx)

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

	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		log.Info("no GitHub token found, the API rate limit will be low",
			"hint", "set GH_TOKEN or GITHUB_TOKEN")
	}
	gh := httpgh.New(token) // GitHub REST client (enrichment)
	clk := clockwork.NewRealClock()
	sc := scrape.NewScraper(log) // HTML scraper for Star Lists
	enr := render.Enricher{GH: gh, Now: clk, Log: log}

	for i, list := range cfg.Lists {
		// 1) scrape owner/repo slugs from your Star List
		var allSlugs []string
		seen := map[string]struct{}{}
		for _, src := range list.Sources {
			log.V(1).Info("scraping star list", "user", src.User, "slug", src.Slug)
			slugs, err := sc.ListRepos(ctx, src.User, src.Slug)
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
				return err
			}
			// Individual lookup failures are reported but do not abort the
			// run: a single deleted repository should not stop the whole list
			// from being regenerated.
			log.Error(err, "some repository lookups failed", "failed", lookupErr.Failed, "total", lookupErr.Total)
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
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "<!-- awesomegen: %s -->\n", filepath.ToSlash(rel)); err != nil {
			return err
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), md); err != nil {
			return err
		}
		return nil
	}
	return writeFileAtomic(root, out, []byte(md))
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
		if prev, ok := seen[p]; ok {
			return nil, fmt.Errorf("lists[%d] and lists[%d] both write to %s; set a distinct output for each list", prev, i, p)
		}
		seen[p] = i
		out[i] = p
	}
	return out, nil
}

// writeFileAtomic writes to a temporary file in the destination directory and
// renames it into place, so an interrupted or failed run never leaves a
// half-written README behind.
//
// The destination has to stay inside root. A symbolic link in the path would
// otherwise redirect the write - and the temporary file that is renamed over
// it - outside of the directory the configuration lives in.
func writeFileAtomic(root, path string, data []byte) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve %s against %s: %w", path, root, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s is outside of %s", path, root)
	}
	if err := rejectSymlinks(root, rel); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".awesomegen-*.tmp")
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
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	return nil
}

// rejectSymlinks verifies that none of the path components below root is a
// symbolic link.
//
// Lexical path validation does not stop a write from following a symlink: a
// directory such as "docs" that points outside of the configuration directory
// would move the generated file out of the allowed root.
func rejectSymlinks(root, rel string) error {
	cur := root
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == "" || seg == "." {
			continue
		}
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
	}
	return nil
}
