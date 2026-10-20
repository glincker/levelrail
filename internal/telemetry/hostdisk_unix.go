//go:build !windows

package telemetry

import (
	"fmt"
	"syscall"
)

// diskSpaceBytes reads path's filesystem capacity via statfs.
func diskSpaceBytes(path string) (total, used int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("telemetry: statfs %q: %w", path, err)
	}
	total, used = diskSpaceFromStatfs(uint64(stat.Blocks), uint64(stat.Bavail), int64(stat.Bsize)) //nolint:gosec // statfs fields are always non-negative in practice
	return total, used, nil
}
