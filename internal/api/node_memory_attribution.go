package api

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	memoryAttributionTTL = 15 * time.Second
	otherContainersTop   = 5
)

// unmanagedContainerUsage is one running container Levelrail does not manage.
type unmanagedContainerUsage struct {
	Name        string  `json:"name"`
	MemoryBytes float64 `json:"memory_bytes"`
}

type hostMemoryAttribution struct {
	Levelrail       float64
	OtherContainers float64
	OtherTop        []unmanagedContainerUsage
}

// splitHostMemory attributes used host memory. System is the remainder after
// every named share, never negative; shares are clamped so the parts never
// exceed the used total.
func splitHostMemory(total, available, services, levelrail, other float64) (lr, oc, system float64) {
	remaining := total - available
	if remaining < 0 {
		remaining = 0
	}
	take := func(v float64) float64 {
		if v < 0 {
			v = 0
		}
		if v > remaining {
			v = remaining
		}
		remaining -= v
		return v
	}
	take(services)
	lr = take(levelrail)
	oc = take(other)
	return lr, oc, remaining
}

type memoryAttributionCache struct {
	mu   sync.Mutex
	at   time.Time
	data hostMemoryAttribution
}

// localMemoryAttribution is cached briefly because it fans out one docker
// stats call per unmanaged container.
func (rt *Router) localMemoryAttribution(ctx context.Context) hostMemoryAttribution {
	c := &rt.memAttribution
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.at.IsZero() && time.Since(c.at) < memoryAttributionTTL {
		return c.data
	}
	out := hostMemoryAttribution{Levelrail: float64(telemetry.SelfRSSBytes())}
	if rt.containers != nil {
		if stats, ok := rt.containers.(docker.StatsInspector); ok {
			if list, err := rt.containers.ListByPrefix(ctx, ""); err == nil {
				if desired, derr := rt.loadOrphanDesired(ctx); derr == nil {
					out.OtherContainers, out.OtherTop = unmanagedUsage(ctx, list, desired, stats)
				}
			}
		}
	}
	c.at = time.Now()
	c.data = out
	return out
}

func unmanagedUsage(ctx context.Context, list []docker.ContainerState, desired orphans.Desired, stats docker.StatsInspector) (float64, []unmanagedContainerUsage) {
	var rows []unmanagedContainerUsage
	var sum float64
	for _, ct := range list {
		if !ct.Running || orphans.IsManaged(ct, desired, "") {
			continue
		}
		s, err := stats.Stats(ctx, ct.ID)
		if err != nil {
			continue
		}
		v := float64(s.MemoryUsageBytes)
		sum += v
		rows = append(rows, unmanagedContainerUsage{Name: ct.Name, MemoryBytes: v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].MemoryBytes > rows[j].MemoryBytes })
	if len(rows) > otherContainersTop {
		rows = rows[:otherContainersTop]
	}
	return sum, rows
}

func (rt *Router) applyMemoryAttribution(ctx context.Context, row *nodeResourceUsageResource) {
	attr := rt.localMemoryAttribution(ctx)
	lr, oc := attr.Levelrail, attr.OtherContainers
	row.LevelrailMemoryBytes = &lr
	row.OtherContainersMemoryBytes = &oc
	row.OtherContainers = attr.OtherTop
	if row.MemoryTotalBytes == nil || row.MemoryAvailableBytes == nil {
		return
	}
	var services float64
	if row.MemoryUsageBytes != nil {
		services = *row.MemoryUsageBytes
	}
	lr, oc, system := splitHostMemory(*row.MemoryTotalBytes, *row.MemoryAvailableBytes, services, attr.Levelrail, attr.OtherContainers)
	row.LevelrailMemoryBytes, row.OtherContainersMemoryBytes, row.SystemMemoryBytes = &lr, &oc, &system
}
