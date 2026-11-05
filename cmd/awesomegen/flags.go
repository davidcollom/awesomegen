package main

import (
	"fmt"
	"slices"
	"strings"

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

// annotationSharedFlags names the annotation that holds the shared flags a
// command honours.
//
// The annotation travels with the command, so the pre-run hook of the root
// command knows what the command it is running does with the flags - also
// for the commands that cobra adds itself, which declare nothing.
const annotationSharedFlags = "awesomegen/shared-flags"

// usesSharedFlags declares the shared flags that the command honours.
//
// A command that does not declare a flag refuses it, so that a flag that is
// ignored cannot slip through.
func usesSharedFlags(cmd *cobra.Command, names ...string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationSharedFlags] = strings.Join(names, ",")
}

// sharedFlagsOf returns the shared flags that the command honours.
//
// A command that declares nothing honours nothing: the commands that cobra
// adds itself are in that group.
func sharedFlagsOf(cmd *cobra.Command) []string {
	declared, ok := cmd.Annotations[annotationSharedFlags]
	if !ok {
		return nil
	}
	if declared == "" {
		return nil
	}
	return strings.Split(declared, ",")
}

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
//
// The flags are shared, so a flag that is written before the name of the
// subcommand belongs to the subcommand as well: "awesomegen --dry-run
// validate" is the same request as "awesomegen validate --dry-run", and it
// is refused for the same reason.
func rejectUnsupportedFlags(cmd *cobra.Command) error {
	supported := sharedFlagsOf(cmd)
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
