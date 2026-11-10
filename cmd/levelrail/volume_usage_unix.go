//go:build !windows

package main

import "syscall"

func filesystemTotalBytes(dir string) int64 {
	var st syscall.Statfs_t
	if syscall.Statfs(dir, &st) != nil {
		return 0
	}
	return int64(st.Blocks) * int64(st.Bsize) //nolint:gosec // block counts of a real filesystem fit int64
}
