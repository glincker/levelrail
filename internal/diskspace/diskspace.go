// Package diskspace provides a single, reusable free-disk-space check
// (statfs's Bavail, the same "free space an unprivileged user could
// actually write" number GET /api/v1/system/doctor's disk_space check
// and GET /api/v1/system/status already surface), so disk-heavy
// operations elsewhere in this codebase (BuildKit builds, backup
// bookkeeping) can gate on it without reimplementing the syscall.
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
