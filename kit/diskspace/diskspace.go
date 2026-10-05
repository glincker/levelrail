// Package diskspace reports free and total bytes of the filesystem holding a
// path, on Unix and Windows, and formats byte counts for display.
package diskspace

import "fmt"

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
