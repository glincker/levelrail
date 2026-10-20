//go:build darwin

package telemetry

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// platformHostMemory reads total memory from hw.memsize and approximates
// "available" as free, speculative and file-backed pages, the reclaimable
// share macOS reports without needing Mach host calls.
func platformHostMemory() (totalBytes, availableBytes int64, err error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, 0, fmt.Errorf("telemetry: sysctl hw.memsize: %w", err)
	}
	pageSize, err := unix.SysctlUint32("hw.pagesize")
	if err != nil {
		return 0, 0, fmt.Errorf("telemetry: sysctl hw.pagesize: %w", err)
	}
	var pages uint64
	for _, key := range []string{"vm.page_free_count", "vm.page_speculative_count", "vm.page_pageable_external_count"} {
		n, err := unix.SysctlUint32(key)
		if err != nil {
			return 0, 0, fmt.Errorf("telemetry: sysctl %s: %w", key, err)
		}
		pages += uint64(n)
	}
	available := pages * uint64(pageSize)
	if available > total {
		available = total
	}
	return int64(total), int64(available), nil //nolint:gosec // physical memory sizes fit in int64
}
