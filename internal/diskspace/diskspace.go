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

// HumanBytes renders bytes the same way web/src/lib/format.ts's own
// formatBytes does (binary units, one decimal place under 10 of a unit,
// none at or above), so a value read from Free and one read from the
// dashboard's own API response read the same regardless of which side
// rendered it. Negative bytes render as "0 B" rather than a negative
// unit, since this only ever wraps a real filesystem free-space value.
func HumanBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := [...]string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit > 0 && value < 10 {
		return fmt.Sprintf("%.1f %s", value, units[unit])
	}
	return fmt.Sprintf("%.0f %s", value, units[unit])
}
