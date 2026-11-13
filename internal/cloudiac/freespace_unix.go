//go:build linux || darwin

package cloudiac

import (
	"fmt"
	"math"
	"syscall"
)

// FreeBytes reports the space an unprivileged process may still write under dir.
func FreeBytes(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("cloudiac: statfs: %w", err)
	}
	avail, size := uint64(st.Bavail), uint64(st.Bsize) //nolint:gosec,unconvert // field types differ per OS
	if size != 0 && avail > math.MaxInt64/size {
		return math.MaxInt64, nil
	}
	return int64(avail * size), nil //nolint:gosec // bounded above
}
