//go:build !windows

package main

import "syscall"

// freeDiskGiB is the space available to the user on the volume holding path.
func freeDiskGiB(path string) int {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0
	}
	return int(uint64(st.Bavail) * uint64(st.Bsize) >> 30) //nolint:gosec // block counts fit in uint64
}
