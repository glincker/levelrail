// Package diskfree reads filesystem usage through one call so callers build on
// every platform.
package diskfree

// Info is the usage of the filesystem that holds a path. Free counts only what
// an unprivileged writer can use. Inode fields are zero where the filesystem
// does not report them.
type Info struct {
	Free        int64
	Total       int64
	InodesFree  uint64
	InodesTotal uint64
}
