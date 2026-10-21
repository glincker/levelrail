package telemetry

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
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

// HostSwapBytes reads swap total and free bytes from /proc/meminfo. A host
// with no /proc/meminfo (macOS) or no swap reports zeros and no error.
func HostSwapBytes() (total, free int64, err error) {
	data, rerr := os.ReadFile("/proc/meminfo")
	if rerr != nil {
		return 0, 0, nil //nolint:nilerr // absent on non-Linux hosts, which simply have no swap reading
	}
	return parseSwap(data)
}

func parseSwap(data []byte) (total, free int64, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "SwapTotal:"):
			if v, perr := parseMemInfoLineKB(line); perr == nil {
				total = v * 1024
			}
		case strings.HasPrefix(line, "SwapFree:"):
			if v, perr := parseMemInfoLineKB(line); perr == nil {
				free = v * 1024
			}
		}
	}
	if free > total {
		free = total
	}
	return total, free, nil
}
