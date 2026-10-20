package telemetry

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// SelfRSSBytes is this process's resident set size. Falls back to the Go
// runtime's footprint where /proc is unavailable.
func SelfRSSBytes() uint64 {
	if data, err := os.ReadFile("/proc/self/statm"); err == nil {
		if rss, ok := parseStatmRSS(string(data), uint64(os.Getpagesize())); ok { //nolint:gosec // page size is always positive
			return rss
		}
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Sys
}

// parseStatmRSS reads the second field (resident pages) of /proc/self/statm.
func parseStatmRSS(statm string, pageSize uint64) (uint64, bool) {
	fields := strings.Fields(statm)
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * pageSize, true
}
