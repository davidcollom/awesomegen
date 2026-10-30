package main

import (
	"fmt"
	"io"
	"runtime"
	"strings"

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
			// The information is built in a buffer and written once, so
			// that a broken pipe is reported as an error instead of
			// being swallowed.
			var b strings.Builder
			fmt.Fprintf(&b, "awesomegen %s\n", version)
			if commit != "" {
				fmt.Fprintf(&b, "commit:     %s\n", commit)
			}
			if date != "" {
				fmt.Fprintf(&b, "built:      %s\n", date)
			}
			fmt.Fprintf(&b, "go:         %s\n", runtime.Version())
			fmt.Fprintf(&b, "platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
			_, err := io.WriteString(cmd.OutOrStdout(), b.String())
			return err
		},
	}
}
