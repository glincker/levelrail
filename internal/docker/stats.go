package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// ContainerStats is one point-in-time resource usage snapshot, trimmed
// and computed down to what TASKS.md 2.1's metrics store actually needs
// (CPU/memory/disk IO/network IO), same shape of simplification
// ContainerState already applies to Docker's raw container summary.
type ContainerStats struct {
	// CPUPercent is 0-100 per core, i.e. a container fully using 2 cores
	// on an otherwise idle host reports 200.0, matching `docker stats`'
	// own convention, computed the same way (delta of cpu_usage over
	// delta of system_cpu_usage, times online CPU count).
	CPUPercent float64
	// MemoryUsageBytes excludes page cache when the cgroup reports it
	// separately (Stats above), matching `docker stats`' "used" figure
	// rather than the raw cgroup usage counter, which double-counts
	// reclaimable cache as "used."
	MemoryUsageBytes uint64
	MemoryLimitBytes uint64
	// NetworkRxBytes/TxBytes are summed across every interface Docker
	// reports for this container; a single container's total network
	// usage is what CLAUDE.md 6's per-app metrics list asks for, not a
	// per-interface breakdown.
	NetworkRxBytes uint64
	NetworkTxBytes uint64
	// DiskReadBytes/WriteBytes are summed across every block device
	// Docker's blkio accounting reports for this container's cgroup.
	DiskReadBytes  uint64
	DiskWriteBytes uint64
}

// Stats fetches one resource-usage snapshot for the container with this
// ID, via Docker's one-shot stats endpoint (a single accurate sample,
// not a subscription): correct for a periodic 15s-resolution collector
// (TASKS.md 2.1), unlike the streaming variant which is built for a
// live-updating `docker stats`-style display instead.
func (c *Client) Stats(ctx context.Context, containerID string) (ContainerStats, error) {
	reader, err := c.cli.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return ContainerStats{}, fmt.Errorf("docker: stats %s: %w", containerID, err)
	}
	defer func() { _ = reader.Body.Close() }()

	var raw container.StatsResponse
	if err := json.NewDecoder(reader.Body).Decode(&raw); err != nil {
		return ContainerStats{}, fmt.Errorf("docker: decode stats %s: %w", containerID, err)
	}

	return ContainerStats{
		CPUPercent:       cpuPercent(raw.CPUStats, raw.PreCPUStats),
		MemoryUsageBytes: memoryUsage(raw.MemoryStats),
		MemoryLimitBytes: raw.MemoryStats.Limit,
		NetworkRxBytes:   sumNetwork(raw.Networks, func(n networkStats) uint64 { return n.RxBytes }),
		NetworkTxBytes:   sumNetwork(raw.Networks, func(n networkStats) uint64 { return n.TxBytes }),
		DiskReadBytes:    sumBlkio(raw.BlkioStats.IoServiceBytesRecursive, "read"),
		DiskWriteBytes:   sumBlkio(raw.BlkioStats.IoServiceBytesRecursive, "write"),
	}, nil
}

// cpuPercent matches the Docker CLI's own calculateCPUPercentUnix
// formula: the fraction of total system CPU time this container
// consumed since the previous sample (one-shot stats populates PreCPU
// from Docker's own internal previous read), scaled by online CPU
// count so a container pinned to 2 cores on an 8-core host still reads
// against its own core budget, not the whole host's.
//
// cgroup v2 caveat, documented rather than silently assumed away: on
// some v2 configurations OnlineCPUs can read 0 even though the field
// exists; that's treated as "1 CPU" (the same fallback the Docker CLI
// itself uses), not a divide-by-zero.
func cpuPercent(current, previous container.CPUStats) float64 {
	cpuDelta := float64(current.CPUUsage.TotalUsage) - float64(previous.CPUUsage.TotalUsage)
	systemDelta := float64(current.SystemUsage) - float64(previous.SystemUsage)
	if cpuDelta <= 0 || systemDelta <= 0 {
		return 0
	}
	onlineCPUs := current.OnlineCPUs
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}
	return (cpuDelta / systemDelta) * float64(onlineCPUs) * 100.0
}

// memoryUsage subtracts page cache from the raw usage counter when the
// cgroup reports it, matching `docker stats`' "used" figure: the raw
// counter otherwise counts reclaimable page cache as if it were memory
// pressure, which overstates what an operator actually needs to worry
// about. cgroup v1 reports this as stats["cache"]; v2 reports it as
// stats["file"] instead. Falling back to the raw usage when neither key
// is present (an empty Stats map, or a cgroup driver that doesn't
// report either) rather than guessing.
func memoryUsage(m container.MemoryStats) uint64 {
	if cache, ok := m.Stats["cache"]; ok && cache < m.Usage {
		return m.Usage - cache
	}
	if file, ok := m.Stats["file"]; ok && file < m.Usage {
		return m.Usage - file
	}
	return m.Usage
}

// networkStats is the subset of container.NetworkStats sumNetwork's
// caller-supplied accessor actually needs, so callers stay decoupled
// from the SDK type's full field list.
type networkStats = container.NetworkStats

func sumNetwork(networks map[string]container.NetworkStats, field func(networkStats) uint64) uint64 {
	var total uint64
	for _, n := range networks {
		total += field(n)
	}
	return total
}

func sumBlkio(entries []container.BlkioStatEntry, op string) uint64 {
	var total uint64
	for _, e := range entries {
		// Docker reports Op as "Read"/"Write" (cgroup v1) or "read"/
		// "write" (cgroup v2 via some drivers); compare case-insensitively
		// rather than assuming one casing.
		if strings.EqualFold(e.Op, op) {
			total += e.Value
		}
	}
	return total
}
