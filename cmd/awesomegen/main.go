// Command awesomegen generates Awesome list README files from GitHub Star
// Lists.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/spf13/cobra"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

// commit is the git commit the binary was built from.
var commit = ""

// date is the build date of the binary.
var date = ""

// loggerKey is the key the logger is stored under in the command context.
type loggerKey struct{}

func main() {
	// Cancel the run on SIGINT/SIGTERM so a scheduled job can be stopped
	// promptly instead of running to completion.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// The deferred stop has to run before the process exits: os.Exit does
	// not run the deferred calls, so the signal handler would stay
	// installed until the process is gone anyway. Restoring the default
	// behaviour explicitly keeps the shutdown order predictable.
	defer stop()

	root := newRootCommand()
	if err := root.ExecuteContext(ctx); err != nil {
		// The error has already been reported by cobra, so the exit code
		// is all that is left to do. stop() is called first, because
		// os.Exit does not run the deferred calls.
		stop()
		os.Exit(1)
	}
}

// options holds the settings that the command line accepts.
type options struct {
	// configPath is the path of the configuration file.
	configPath string
	// verbose turns on the detailed logging.
	verbose bool
	// dryRun renders the README files and prints them instead of writing
	// them to disk.
	dryRun bool
}

// newRootCommand builds the command tree of the CLI.
//
// The root command generates the README files, so that the plain
// "awesomegen" invocation keeps working, and it also holds the subcommands
// for the other tasks.
func newRootCommand() *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:   "awesomegen",
		Short: "Generate Awesome list README(s) from GitHub Star Lists",
		Long: `awesomegen turns a public GitHub Star List into an Awesome list README.

It scrapes the Star List, enriches every repository with the data of the
GitHub API, filters the result and renders an Awesome formatted README.

Run "awesomegen validate" to check a configuration file without contacting
GitHub, and "awesomegen schema" to print the JSON schema of the
configuration file.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: false,
		Args:          cobra.NoArgs,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			if opts.verbose {
				// The detailed messages are logged at level 1, so they
				// only appear when they are asked for.
				stdr.SetVerbosity(1)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), loggerKey{}, newLogger()))
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGenerate(cmd, opts)
		},
	}

	root.PersistentFlags().StringVarP(&opts.configPath, "config", "c", "config.yaml", "Path to the configuration file")
	root.PersistentFlags().BoolVar(&opts.verbose, "verbose", false, "Log what is happening in more detail")
	root.PersistentFlags().BoolVar(&opts.dryRun, "dry-run", false, "Print the rendered README(s) instead of writing them")

	root.AddCommand(newGenerateCommand(opts))
	root.AddCommand(newValidateCommand(opts))
	root.AddCommand(newSchemaCommand())
	root.AddCommand(newVersionCommand())

	return root
}

// newLogger returns the logger of the CLI. The messages go to the standard
// error stream, so that the standard output stream can carry the rendered
// README.
func newLogger() logr.Logger {
	return stdr.New(nil)
}

// loggerFrom returns the logger of the command.
func loggerFrom(ctx context.Context) logr.Logger {
	if l, ok := ctx.Value(loggerKey{}).(logr.Logger); ok {
		return l
	}
	return logr.Discard()
}
