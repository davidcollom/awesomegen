package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/go-logr/stdr"
	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"

	"github.com/davidcollom/awesomegen/internal/config"
	httpgh "github.com/davidcollom/awesomegen/internal/github"
	"github.com/davidcollom/awesomegen/internal/render"
	"github.com/davidcollom/awesomegen/internal/scrape"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log := stdr.New(nil)
	log.WithName("awesomegen").Info("starting awesomegen")

	// Cancel the run on SIGINT/SIGTERM so a scheduled job can be stopped
	// promptly instead of running to completion.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var cfgPath string
	cmd := &cobra.Command{
		Use:           "awesomegen",
		Short:         "Generate Awesome list README(s) from GitHub Star Lists",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}

			// Resolve every output path up front so two lists cannot
			// silently overwrite each other.
			outputs, err := resolveOutputs(cfg, cfgPath)
			if err != nil {
				return err
			}

			token := os.Getenv("GH_TOKEN")
			if token == "" {
				token = os.Getenv("GITHUB_TOKEN")
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
					slugs, err := sc.ListRepos(ctx, src.User, src.Slug)
					if err != nil {
						return fmt.Errorf("scrape %s/%s: %w", src.User, src.Slug, err)
					}
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
					// Individual lookup failures are reported but do not
					// abort the run: a single deleted repository should not
					// stop the whole list from being regenerated.
					log.Error(err, "some repository lookups failed", "failed", lookupErr.Failed, "total", lookupErr.Total)
				}

				// 3) render markdown
				md, err := render.Markdown(enriched)
				if err != nil {
					return err
				}

				// Refuse to replace an existing README with an empty one.
				// A failing API lookup or an expired token silently drops
				// every repository, and committing that empty result would
				// wipe out a curated list.
				if countItems(enriched) == 0 {
					return fmt.Errorf("list %q produced no entries; refusing to overwrite %s with an empty README", list.Slug, outputs[i])
				}

				out := outputs[i]
				if err := writeFileAtomic(out, []byte(md)); err != nil {
					return err
				}
				log.Info("wrote README", "file", out, "title", list.Title)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "Path to config file")
	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

// countItems returns the number of items across every category of a list.
func countItems(l config.List) int {
	n := 0
	for _, c := range l.Categories {
		if c == nil {
			continue
		}
		n += len(c.Items)
	}
	return n
}

// resolveOutputs determines the output path for every list. Relative paths
// are resolved against the directory holding the config file so the result
// does not depend on the working directory.
func resolveOutputs(cfg config.Config, cfgPath string) ([]string, error) {
	base, err := filepath.Abs(filepath.Dir(cfgPath))
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
func writeFileAtomic(path string, data []byte) error {
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
