package backup

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Scratch container hardening for restore drills.
const (
	EnvDrillMemoryMB = "APP_BACKUP_DRILL_MEMORY_MB"
	EnvDrillCPUMilli = "APP_BACKUP_DRILL_CPU_MILLI"

	DefaultDrillMemoryMB = 1024
	DefaultDrillCPUMilli = 1000

	drillScratchLabel  = "drill.scratch"
	drillScratchPrefix = "drill-"
	drillNetworkNone   = "none"
)

// DrillLimitsFromEnv returns the memory (bytes) and CPU (nano CPUs) caps for
// a scratch container.
func DrillLimitsFromEnv(lookup func(string) (string, bool)) (memoryBytes, nanoCPUs int64) {
	read := func(name string, def int64) int64 {
		if v, ok := lookup(name); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
				return n
			}
		}
		return def
	}
	return read(EnvDrillMemoryMB, DefaultDrillMemoryMB) << 20, read(EnvDrillCPUMilli, DefaultDrillCPUMilli) * 1_000_000
}

// hardenScratch caps resources, removes the network and labels the container
// so an orphan sweep can find it.
func (r *DrillRunner) hardenScratch(spec *docker.ContainerSpec) {
	mem, cpu := r.ScratchMemoryBytes, r.ScratchNanoCPUs
	if mem <= 0 {
		mem = DefaultDrillMemoryMB << 20
	}
	if cpu <= 0 {
		cpu = DefaultDrillCPUMilli * 1_000_000
	}
	spec.Resources = &docker.Resources{MemoryBytes: mem, NanoCPUs: cpu, SwapMemoryBytes: mem}
	spec.NetworkMode = drillNetworkNone
	spec.Network = nil
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	spec.Labels[drillScratchLabel] = "1"
}

// SweepOrphans removes scratch containers left by a crashed drill and
// returns how many it removed.
func (r *DrillRunner) SweepOrphans(ctx context.Context) (int, error) {
	if r.Runtime == nil {
		return 0, nil
	}
	list, err := r.Runtime.ListByPrefix(ctx, drillScratchPrefix)
	if err != nil {
		return 0, fmt.Errorf("backup: list drill scratch containers: %w", err)
	}
	removed := 0
	var firstErr error
	for _, c := range list {
		if c.Labels[drillScratchLabel] != "1" {
			continue
		}
		if err := r.Runtime.Remove(ctx, c.ID, true); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("backup: remove orphan drill container %q: %w", c.Name, err)
			}
			continue
		}
		removed++
		r.log().Info("backup: removed orphan drill container", slog.String("container", c.Name))
	}
	return removed, firstErr
}
