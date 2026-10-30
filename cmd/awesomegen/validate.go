package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/davidcollom/awesomegen/internal/config"
)

// newValidateCommand builds the command that checks a configuration file.
//
// The check is offline: the configuration is parsed and validated, the
// output paths are resolved and the sources are listed. Nothing is scraped
// and no request is sent to GitHub, so the command can be used in a
// pre-commit hook or in a pull request check.
func newValidateCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check the configuration file without contacting GitHub",
		Long: `Check the configuration file without contacting GitHub.

The configuration is parsed with the same loader the generator uses, so a
configuration that passes this check is one the generator accepts. The
output paths are resolved as well, which catches the cases where two lists
would write to the same file or where an output path would leave the
directory of the configuration file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runValidate(cmd, opts)
		},
	}
}

// runValidate checks the configuration and prints a summary of it.
func runValidate(cmd *cobra.Command, opts *options) error {
	log := loggerFrom(cmd.Context())

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}

	outputs, err := resolveOutputs(cfg, opts.configPath)
	if err != nil {
		return err
	}

	root, err := configRoot(opts.configPath)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s is valid\n", opts.configPath)
	fmt.Fprintf(out, "%d list(s)\n", len(cfg.Lists))

	for i, list := range cfg.Lists {
		name := list.Slug
		if name == "" {
			name = list.Title
		}
		fmt.Fprintf(out, "\n- %s\n", name)
		fmt.Fprintf(out, "  title:       %s\n", list.Title)
		for _, src := range list.Sources {
			fmt.Fprintf(out, "  source:      %s/%s\n", src.User, src.Slug)
		}
		fmt.Fprintf(out, "  output:      %s\n", relativeTo(root, outputs[i]))
		fmt.Fprintf(out, "  min_stars:   %d\n", list.MinStars)
		fmt.Fprintf(out, "  stale:       %d month(s)\n", list.StaleMonthsValue())
		fmt.Fprintf(out, "  stars:       %s\n", starsFormatName(list))
		fmt.Fprintf(out, "  topics:      %s\n", topicGroupingName(list))
		items := 0
		for _, c := range list.Categories {
			if c != nil {
				items += len(c.Items)
			}
		}
		fmt.Fprintf(out, "  entries:     %d curated\n", items)
	}

	log.V(1).Info("configuration is valid", "lists", len(cfg.Lists))
	return nil
}

// relativeTo returns path relative to root, or path itself when it is not
// below root.
func relativeTo(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// starsFormatName returns the name of the star format of the list.
func starsFormatName(list config.List) string {
	if list.StarsFormat == nil {
		return string(config.StarsFormatLocale)
	}
	return string(*list.StarsFormat)
}

// topicGroupingName returns the name of the topic grouping mode of the list.
func topicGroupingName(list config.List) string {
	if list.TopicGroupingMode == nil {
		return string(config.TopicGroupingModeFlat)
	}
	return string(*list.TopicGroupingMode)
}
