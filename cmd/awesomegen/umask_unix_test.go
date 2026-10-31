//go:build !windows

package main

import (
	"syscall"
	"testing"
)

// setUmask sets the umask of the process and restores the previous one when
// the test is done.
//
// The umask is a property of the whole process, so a test that changes it
// must not run in parallel with the other tests.
func setUmask(t *testing.T, mask int) {
	t.Helper()

	previous := syscall.Umask(mask)
	t.Cleanup(func() {
		syscall.Umask(previous)
	})
}
