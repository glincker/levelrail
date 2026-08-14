package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestCPUPercent(t *testing.T) {
	tests := []struct {
		name     string
		current  container.CPUStats
		previous container.CPUStats
		want     float64
	}{
		{
			name:     "typical: 25% of one core on a 4-CPU host",
			current:  container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 2_000_000_000}, SystemUsage: 40_000_000_000, OnlineCPUs: 4},
			previous: container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 1_000_000_000}, SystemUsage: 36_000_000_000},
			want:     ((2_000_000_000.0 - 1_000_000_000.0) / (40_000_000_000.0 - 36_000_000_000.0)) * 4 * 100.0,
		},
		{
			name:     "no time elapsed: system delta zero, avoid divide by zero",
			current:  container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 1_000}, SystemUsage: 5_000, OnlineCPUs: 2},
			previous: container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 1_000}, SystemUsage: 5_000},
			want:     0,
		},
		{
			name:     "OnlineCPUs unset (0): falls back to 1, not a divide-by-zero or a zeroed-out result",
			current:  container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 2_000}, SystemUsage: 10_000, OnlineCPUs: 0},
			previous: container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 1_000}, SystemUsage: 9_000},
			// cpuDelta == systemDelta == 1000, so the ratio is exactly 1;
			// times the 1-CPU fallback, times 100, is exactly 100.
			want: 100.0,
		},
		{
			name:     "negative delta (counter reset): clamped to zero, not negative",
			current:  container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 500}, SystemUsage: 10_000, OnlineCPUs: 1},
			previous: container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 1_000}, SystemUsage: 9_000},
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cpuPercent(tt.current, tt.previous); got != tt.want {
				t.Errorf("cpuPercent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryUsage(t *testing.T) {
	tests := []struct {
		name string
		m    container.MemoryStats
		want uint64
	}{
		{
			name: "cgroup v1: subtracts cache from usage",
			m:    container.MemoryStats{Usage: 1000, Stats: map[string]uint64{"cache": 300}},
			want: 700,
		},
		{
			name: "cgroup v2: subtracts file from usage",
			m:    container.MemoryStats{Usage: 1000, Stats: map[string]uint64{"file": 200}},
			want: 800,
		},
		{
			name: "neither key present: falls back to raw usage",
			m:    container.MemoryStats{Usage: 1000, Stats: map[string]uint64{}},
			want: 1000,
		},
		{
			name: "cache larger than usage (shouldn't happen, but don't underflow): falls back to raw usage",
			m:    container.MemoryStats{Usage: 100, Stats: map[string]uint64{"cache": 500}},
			want: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := memoryUsage(tt.m); got != tt.want {
				t.Errorf("memoryUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSumNetwork(t *testing.T) {
	networks := map[string]container.NetworkStats{
		"eth0": {RxBytes: 100, TxBytes: 50},
		"eth1": {RxBytes: 200, TxBytes: 75},
	}

	if got := sumNetwork(networks, func(n networkStats) uint64 { return n.RxBytes }); got != 300 {
		t.Errorf("sumNetwork(RxBytes) = %d, want 300 (summed across interfaces)", got)
	}
	if got := sumNetwork(networks, func(n networkStats) uint64 { return n.TxBytes }); got != 125 {
		t.Errorf("sumNetwork(TxBytes) = %d, want 125 (summed across interfaces)", got)
	}
	if got := sumNetwork(nil, func(n networkStats) uint64 { return n.RxBytes }); got != 0 {
		t.Errorf("sumNetwork(nil) = %d, want 0", got)
	}
}

func TestSumBlkio(t *testing.T) {
	entries := []container.BlkioStatEntry{
		{Op: "Read", Value: 1000},
		{Op: "Write", Value: 500},
		{Op: "Read", Value: 200}, // multiple devices: both Read entries sum
		{Op: "Sync", Value: 999}, // an op we don't track: must not leak into either total
	}

	if got := sumBlkio(entries, "read"); got != 1200 {
		t.Errorf("sumBlkio(read) = %d, want 1200", got)
	}
	if got := sumBlkio(entries, "write"); got != 500 {
		t.Errorf("sumBlkio(write) = %d, want 500", got)
	}
}
