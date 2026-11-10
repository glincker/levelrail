//go:build !windows

package main

import (
	"os"
	"syscall"
)

// openJournalNoFollow never follows a symlink and never blocks on a FIFO swapped in after the Lstat.
func openJournalNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0) //nolint:gosec // operator-supplied journal path, re-checked with fstat by the caller
}
