//go:build !windows

package telemetry

import (
	"fmt"
	"syscall"
)

// HostInodes reads path's filesystem inode total and free counts. Filesystems
// without inode accounting report zero totals.
func HostInodes(path string) (total, free uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("telemetry: statfs %q: %w", path, err)
	}
	return uint64(st.Files), uint64(st.Ffree), nil //nolint:unconvert // field width differs per platform
}
