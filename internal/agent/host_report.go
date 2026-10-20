package agent

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	hostReportInterval = 30 * time.Second
	// hostReportMaxAge is how long a node's last report stays usable, a few
	// missed ticks, so a silent agent falls back to the plain row shape.
	hostReportMaxAge = 3 * time.Minute
	otherTopN        = 5
)

// UnmanagedContainer is one running container on a node that Levelrail
// does not manage.
type UnmanagedContainer struct {
	Name        string
	MemoryBytes uint64
}

// HostReport is one node's host memory split as the agent reads it.
// MemoryTotalBytes, DiskTotalBytes and DiskFreeBytes are zero when unreadable.
type HostReport struct {
	MemoryTotalBytes           uint64
	MemoryAvailableBytes       uint64
	AgentRSSBytes              uint64
	OtherContainersMemoryBytes uint64
	OtherContainers            []UnmanagedContainer
	CPUCores                   uint32
	DiskTotalBytes             uint64
	DiskFreeBytes              uint64
	DiskInodesTotal            uint64
	DiskInodesFree             uint64
	SwapTotalBytes             uint64
	SwapFreeBytes              uint64
}

// HostProbe reads the node's current host report.
type HostProbe func(ctx context.Context) HostReport

// WithHostProbe makes RunSession report this node's host memory split to
// the control plane periodically.
func WithHostProbe(p HostProbe) SessionOption {
	return func(c *sessionConfig) { c.hostProbe = p }
}

// NewHostProbe reads host memory, this process's RSS and the unmanaged
// running containers rt can see. A runtime without stats support yields no
// container breakdown.
func NewHostProbe(rt docker.Runtime, diskPath string) HostProbe {
	return func(ctx context.Context) HostReport {
		out := HostReport{AgentRSSBytes: telemetry.SelfRSSBytes(), CPUCores: uint32(max(runtime.NumCPU(), 0))} //nolint:gosec // NumCPU is small and positive
		if total, used, err := telemetry.HostDiskBytes(diskPath); err == nil && total > 0 {
			out.DiskTotalBytes = uint64(total)
			out.DiskFreeBytes = uint64(max(total-used, 0))
		}
		if total, free, err := telemetry.HostInodes(diskPath); err == nil {
			out.DiskInodesTotal, out.DiskInodesFree = total, free
		}
		if total, free, err := telemetry.HostSwapBytes(); err == nil && total > 0 {
			out.SwapTotalBytes, out.SwapFreeBytes = uint64(total), uint64(max(free, 0))
		}
		if total, avail, err := telemetry.HostMemoryBytes(); err == nil && total > 0 {
			out.MemoryTotalBytes = uint64(total)
			out.MemoryAvailableBytes = uint64(max(avail, 0))
		}
		stats, ok := rt.(docker.StatsInspector)
		if !ok {
			return out
		}
		list, err := rt.ListByPrefix(ctx, "")
		if err != nil {
			return out
		}
		out.OtherContainersMemoryBytes, out.OtherContainers = unmanagedContainerUsage(ctx, list, stats)
		return out
	}
}

// unmanagedContainerUsage sums running containers that carry no Levelrail
// instance label. The agent has no desired state, so the label is the whole
// managed test here.
func unmanagedContainerUsage(ctx context.Context, list []docker.ContainerState, stats docker.StatsInspector) (uint64, []UnmanagedContainer) {
	var rows []UnmanagedContainer
	var sum uint64
	for _, ct := range list {
		if !ct.Running || ct.Labels[spec.InstanceLabelKey] != "" {
			continue
		}
		s, err := stats.Stats(ctx, ct.ID)
		if err != nil {
			continue
		}
		sum += s.MemoryUsageBytes
		rows = append(rows, UnmanagedContainer{Name: ct.Name, MemoryBytes: s.MemoryUsageBytes})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].MemoryBytes > rows[j].MemoryBytes })
	if len(rows) > otherTopN {
		rows = rows[:otherTopN]
	}
	return sum, rows
}

func hostReportLoop(ctx context.Context, send func(*agentpb.AgentMessage), probe HostProbe, interval time.Duration, done <-chan struct{}) {
	report := func() {
		send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_HostReport{HostReport: hostReportToPB(probe(ctx))}})
	}
	report()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}

func hostReportToPB(r HostReport) *agentpb.HostReport {
	out := &agentpb.HostReport{
		MemoryTotalBytes:           r.MemoryTotalBytes,
		MemoryAvailableBytes:       r.MemoryAvailableBytes,
		AgentRssBytes:              r.AgentRSSBytes,
		OtherContainersMemoryBytes: r.OtherContainersMemoryBytes,
		CpuCores:                   r.CPUCores,
		DiskTotalBytes:             r.DiskTotalBytes,
		DiskFreeBytes:              r.DiskFreeBytes,
		DiskInodesTotal:            r.DiskInodesTotal,
		DiskInodesFree:             r.DiskInodesFree,
		SwapTotalBytes:             r.SwapTotalBytes,
		SwapFreeBytes:              r.SwapFreeBytes,
	}
	for _, c := range r.OtherContainers {
		out.OtherContainers = append(out.OtherContainers, &agentpb.UnmanagedContainer{Name: c.Name, MemoryBytes: c.MemoryBytes})
	}
	return out
}

func hostReportFromPB(r *agentpb.HostReport) HostReport {
	out := HostReport{
		MemoryTotalBytes:           r.GetMemoryTotalBytes(),
		MemoryAvailableBytes:       r.GetMemoryAvailableBytes(),
		AgentRSSBytes:              r.GetAgentRssBytes(),
		OtherContainersMemoryBytes: r.GetOtherContainersMemoryBytes(),
		CPUCores:                   r.GetCpuCores(),
		DiskTotalBytes:             r.GetDiskTotalBytes(),
		DiskFreeBytes:              r.GetDiskFreeBytes(),
		DiskInodesTotal:            r.GetDiskInodesTotal(),
		DiskInodesFree:             r.GetDiskInodesFree(),
		SwapTotalBytes:             r.GetSwapTotalBytes(),
		SwapFreeBytes:              r.GetSwapFreeBytes(),
	}
	for _, c := range r.GetOtherContainers() {
		out.OtherContainers = append(out.OtherContainers, UnmanagedContainer{Name: c.GetName(), MemoryBytes: c.GetMemoryBytes()})
	}
	return out
}

// HostReportSink receives a node's host report on the control plane.
type HostReportSink interface {
	SetNodeHostReport(nodeID string, r HostReport)
}

// WithHostReportSink records the host reports agents send up their Session
// stream.
func WithHostReportSink(sink HostReportSink) Option {
	return func(s *Server) { s.hostSink = sink }
}

// HostReportStore keeps each node's latest host report in memory only: it
// is a live gauge, and a restart simply waits for the next agent tick.
type HostReportStore struct {
	mu      sync.Mutex
	now     func() time.Time
	reports map[string]storedHostReport
}

type storedHostReport struct {
	at     time.Time
	report HostReport
}

// NewHostReportStore returns an empty store.
func NewHostReportStore() *HostReportStore {
	return &HostReportStore{now: time.Now, reports: make(map[string]storedHostReport)}
}

// SetNodeHostReport implements HostReportSink.
func (s *HostReportStore) SetNodeHostReport(nodeID string, r HostReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports[nodeID] = storedHostReport{at: s.now(), report: r}
}

// Get returns nodeID's latest report, false when none arrived or it is stale.
func (s *HostReportStore) Get(nodeID string) (HostReport, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.reports[nodeID]
	if !ok || s.now().Sub(st.at) > hostReportMaxAge {
		return HostReport{}, false
	}
	return st.report, true
}
