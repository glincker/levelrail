//go:build !windows

package backup

import "syscall"

// freeBytes reports the space available to unprivileged writers under dir.
func freeBytes(dir string) (uint64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, false
	}
	return uint64(st.Bavail) * uint64(st.Bsize), true //nolint:gosec // both are non-negative
}
