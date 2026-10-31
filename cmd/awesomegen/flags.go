package main

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
)

// The names of the flags that the root command shares with its
// subcommands.
//
// They are persistent flags of the root command, so every subcommand
// accepts them on the command line, whether it uses them or not.
const (
	flagConfig  = "config"
	flagVerbose = "verbose"
	flagDryRun  = "dry-run"
)

// sharedFlags are the flags that the root command offers to every command of
// the tree.
//
// A flag that is added here is refused by the commands that do not declare
// it, so a new flag cannot be ignored by accident.
//
// The verbose flag is not one of them: it is read by the pre-run hook of the
// root command, which every command inherits, so it always has an effect.
var sharedFlags = []string{flagConfig, flagDryRun}

// rejectUnsupportedFlags fails when the command was given one of the shared
// flags that it does not use.
//
// The flags of the root command are persistent, so every subcommand accepts
// them. A flag that is ignored is a flag that lies: "awesomegen validate
// --dry-run" looks like it rehearses the run, and it does not. The command
// refuses the flag instead of ignoring it, and the message names the flag
// and the command so that the user knows what to change.
//
// Only the flags that were written on the command line are refused: a flag
// that keeps its default is not a request the command has to honour.
func rejectUnsupportedFlags(cmd *cobra.Command, supported ...string) error {
	for _, name := range sharedFlags {
		if slices.Contains(supported, name) {
			continue
		}
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s is not used by the %s command", name, cmd.Name())
		}
	}
	return nil
}
