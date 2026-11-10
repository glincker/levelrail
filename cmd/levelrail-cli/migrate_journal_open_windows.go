//go:build windows

package main

import "os"

// openJournalNoFollow opens the journal; the caller's Lstat and fstat SameFile check refuse a swapped file.
func openJournalNoFollow(path string) (*os.File, error) {
	return os.Open(path) //nolint:gosec // operator-supplied journal path, re-checked with fstat by the caller
}
