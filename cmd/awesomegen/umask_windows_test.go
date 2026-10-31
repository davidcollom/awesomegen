//go:build windows

package main

import "testing"

// setUmask skips the test: Windows has no umask.
//
// The permissions of a file on Windows are not derived from a mask of the
// process, so the mode of a new file cannot be checked there.
func setUmask(t *testing.T, _ int) {
	t.Helper()

	t.Skip("Windows has no umask")
}
