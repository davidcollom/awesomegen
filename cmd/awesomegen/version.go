package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// newVersionCommand builds the command that prints the build information of
// the binary.
//
// The version, the commit and the build date are stamped into the binary at
// build time. The commit and the date are empty for a binary that was built
// without them - "go run", for example - and are left out of the output
// then.
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of awesomegen",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "awesomegen %s\n", version)
			if commit != "" {
				fmt.Fprintf(out, "commit:     %s\n", commit)
			}
			if date != "" {
				fmt.Fprintf(out, "built:      %s\n", date)
			}
			fmt.Fprintf(out, "go:         %s\n", runtime.Version())
			fmt.Fprintf(out, "platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
}
