//go:build !windows

package diskspace

import (
	"fmt"
	"syscall"
)

// Free reports the bytes free at path that an unprivileged user could
// write, per statfs's Bavail field (not Bfree, which includes
// root-reserved blocks).
func Free(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("diskspace: statfs %q: %w", path, err)
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil //nolint:gosec // statfs fields are always non-negative in practice
}

// Usage reports free (Bavail) and total bytes of the filesystem holding path.
func Usage(path string) (free, total int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("diskspace: statfs %q: %w", path, err)
	}
	bsize := int64(stat.Bsize)
	return int64(stat.Bavail) * bsize, int64(stat.Blocks) * bsize, nil //nolint:gosec // statfs fields are always non-negative in practice
}
