package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/davidcollom/awesomegen"
)

// newSchemaCommand builds the command that prints the JSON schema of the
// configuration file.
//
// The schema is embedded in the binary, so printing it needs neither a
// network connection nor the schema generator.
func newSchemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON schema of the configuration file",
		Long: `Print the JSON schema of the configuration file.

The schema is the one that the configuration file is checked against in an
editor. It is embedded in the binary, so this command works offline.`,
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// The schema is embedded in the binary: the command reads
			// no configuration file and writes nothing, so both flags
			// would only pretend to do something.
			return rejectUnsupportedFlags(cmd, flagVerbose)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The schema is written verbatim, so that it can be piped
			// into a file or a validator without being altered.
			if _, err := fmt.Fprint(cmd.OutOrStdout(), string(awesomegen.ConfigSchema)); err != nil {
				return err
			}
			return nil
		},
	}
}
