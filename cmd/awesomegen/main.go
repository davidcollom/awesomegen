package main

import (
	"context"
	"os"

	"github.com/go-logr/stdr"
	"github.com/jonboulle/clockwork"
	"github.com/spf13/cobra"

	"github.com/davidcollom/awesomegen/internal/config"
	httpgh "github.com/davidcollom/awesomegen/internal/github"
	"github.com/davidcollom/awesomegen/internal/render"
	"github.com/davidcollom/awesomegen/internal/scrape"
)

func main() {
	log := stdr.New(nil)

	var cfgPath string
	cmd := &cobra.Command{
		Use:   "awesomegen",
		Short: "Generate Awesome list README(s) from GitHub Star Lists",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return err
			}

			token := os.Getenv("GITHUB_TOKEN")
			gh := httpgh.New(token) // GitHub REST client (enrichment)
			clk := clockwork.NewRealClock()
			sc := scrape.NewScraper(log) // HTML scraper for Star Lists
			enr := render.Enricher{GH: gh, Now: clk, Log: log}

			for _, list := range cfg.Lists {
				// 1) scrape owner/repo slugs from your Star List
				slugs, err := sc.ListRepos(ctx, cfg.User, list.Slug)
				if err != nil {
					return err
				}
				list.SeedRepos(slugs)

				// 2) enrich + filter
				enriched, err := enr.EnrichAndFilter(ctx, list)
				if err != nil {
					return err
				}

				// 3) render markdown
				md := render.Markdown(enriched)
				out := list.Output
				if out == "" {
					out = "README.md"
				}
				if err := os.WriteFile(out, []byte(md), 0o644); err != nil {
					return err
				}
				log.Info("wrote README", "file", out, "title", list.Title)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "Path to config file")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
