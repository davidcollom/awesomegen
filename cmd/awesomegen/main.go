// Command awesomegen generates Awesome list README files from GitHub Star
// Lists.
package main

import (
	"context"
	"fmt"
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

// date is the date of the commit the binary was built from.
//
// It is the date of the commit rather than the date of the build, so that
// two builds of the same commit produce the same binary.
var date = ""

// loggerKey is the key the logger is stored under in the command context.
type loggerKey struct{}

// The names of the command and of the flag that cobra uses for the
// completion scripts.
const (
	// completionCommandName is the name of the command that writes the
	// completion script of a shell.
	completionCommandName = "completion"
	// completionNoDescFlag is the flag that switches the descriptions of
	// the completion script off.
	completionNoDescFlag = "no-descriptions"
)

func init() {
	// Cobra runs only the closest persistent pre-run hook of a command, so
	// a subcommand that has a hook of its own would shadow the one of the
	// root command - and the check of the flags would stop running for it.
	// The traversal makes every hook of the chain run, so the check of the
	// root command keeps its hold on the whole tree.
	cobra.EnableTraverseRunHooks = true
}

func main() {
	// Cancel the run on SIGINT/SIGTERM so a scheduled job can be stopped
	// promptly instead of running to completion.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	os.Exit(runCLI(ctx, newRootCommand(), stop))
}

func runCLI(ctx context.Context, root *cobra.Command, stop context.CancelFunc) int {
	// Restore normal signal handling before main exits the process.
	defer stop()

	if err := root.ExecuteContext(ctx); err != nil {
		// The error has already been reported by cobra, so the exit code
		// is all that is left to do.
		return 1
	}
	return 0
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
		// The pre-run hook of the root command runs for every command of
		// the tree, so this is the place where a flag that a command does
		// not use is refused - also for the commands that cobra adds
		// itself. The traversal of the hooks is switched on in the init
		// function of this file, so that a subcommand with a hook of its
		// own does not shadow this one.
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if opts.verbose {
				// The detailed messages are logged at level 1, so they
				// only appear when they are asked for.
				stdr.SetVerbosity(1)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), loggerKey{}, newLogger()))
			return rejectUnsupportedFlags(cmd)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGenerate(cmd, opts)
		},
	}

	// The root command generates the README files, so it uses every shared
	// flag.
	usesSharedFlags(root, flagConfig, flagDryRun)

	root.PersistentFlags().StringVarP(&opts.configPath, flagConfig, "c", "config.yaml", "Path to the configuration file")
	root.PersistentFlags().BoolVar(&opts.verbose, flagVerbose, false, "Log what is happening in more detail")
	root.PersistentFlags().BoolVar(&opts.dryRun, flagDryRun, false, "Print the rendered README(s) instead of writing them")

	root.AddCommand(newGenerateCommand(opts))
	root.AddCommand(newValidateCommand(opts))
	root.AddCommand(newSchemaCommand())
	root.AddCommand(newVersionCommand())

	// The completion command of cobra only prints its help, so it has no
	// run function and cobra does not run the pre-run hook of the root
	// command for it. The hook is what refuses the flags that a command
	// does not use, so the command is given a run function that prints the
	// help, exactly as before.
	//
	// The shell commands of the completion command write the script to the
	// writer they find when they are built, so they are re-pointed at the
	// writer of the command that runs them.
	root.InitDefaultCompletionCmd()
	configureCompletionCommand(root)

	return root
}

// configureCompletionCommand makes the completion command of cobra behave
// like the other commands of the tree.
//
// The command is runnable, so that the pre-run hook of the root command runs
// for it and the flags that it does not use are refused. Running it prints
// its help, exactly as it did before.
//
// Cobra writes the completion script to the writer that it finds when the
// command is built, so the script would end up in the standard output stream
// even when the caller has set another writer. The run functions of the
// shell commands are therefore replaced with ones that use the writer of the
// command that runs them.
func configureCompletionCommand(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		if cmd.Name() != completionCommandName {
			continue
		}
		cmd.RunE = func(c *cobra.Command, _ []string) error {
			return c.Help()
		}
		for _, shell := range cmd.Commands() {
			shell.RunE = completionRunner(shell.Name())
		}
	}
}

// completionRunner returns the run function of the shell command that writes
// the completion script of that shell.
//
// The writer is looked up when the command runs and not when it is built, so
// that the script goes to the writer of the caller.
func completionRunner(shell string) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, _ []string) error {
		out := c.OutOrStdout()
		// The flag is missing when the descriptions are switched off for
		// good, and then the script carries them.
		noDesc, err := c.Flags().GetBool(completionNoDescFlag)
		if err != nil {
			noDesc = false
		}
		switch shell {
		case "bash":
			return c.Root().GenBashCompletionV2(out, !noDesc)
		case "zsh":
			if noDesc {
				return c.Root().GenZshCompletionNoDesc(out)
			}
			return c.Root().GenZshCompletion(out)
		case "fish":
			return c.Root().GenFishCompletion(out, !noDesc)
		case "powershell":
			if noDesc {
				return c.Root().GenPowerShellCompletion(out)
			}
			return c.Root().GenPowerShellCompletionWithDesc(out)
		default:
			return fmt.Errorf("unknown shell %q", shell)
		}
	}
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
