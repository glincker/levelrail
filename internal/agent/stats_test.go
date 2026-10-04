package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// statsRuntime is transport_test.go's minimal fakeRuntime plus the
// optional docker.StatsInspector capability, standing in for the
// *docker.Client a real agent holds on its own node.
type statsRuntime struct {
	fakeRuntime
	stats       docker.ContainerStats
	err         error
	lastID      string
	lastWasReal bool
}

func (f *statsRuntime) Stats(_ context.Context, containerID string) (docker.ContainerStats, error) {
	f.lastID = containerID
	f.lastWasReal = true
	return f.stats, f.err
}

func TestGRPCTransport_Stats_RoundTrips(t *testing.T) {
	rt := &statsRuntime{stats: docker.ContainerStats{
		CPUPercent:       12.5,
		CPURaw:           docker.CPUStatsRaw{TotalUsageNanos: 10, SystemUsageNanos: 100, OnlineCPUs: 2},
		MemoryUsageBytes: 1024,
		MemoryLimitBytes: 2048,
		NetworkRxBytes:   3,
		NetworkTxBytes:   4,
		DiskReadBytes:    5,
		DiskWriteBytes:   6,
	}}
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, rt)

	// The type assertion multiNodeStatsSource itself makes
	// (cmd/levelrail/main.go), on the same static type a remote node's
	// transport is handed.
	var runtime docker.Runtime = tr
	inspector, ok := runtime.(docker.StatsInspector)
	if !ok {
		t.Fatal("GRPCTransport is not a docker.StatsInspector: a remote node's containers would never report metrics")
	}

	got, err := inspector.Stats(context.Background(), "web-1")
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if rt.lastID != "web-1" {
		t.Errorf("agent saw container id = %q, want web-1", rt.lastID)
	}
	if got != rt.stats {
		t.Errorf("Stats() = %+v, want %+v unchanged", got, rt.stats)
	}
}

// TestGRPCTransport_Stats_RuntimeWithoutCapability proves an agent too
// old (or whose runtime can't report stats) degrades to a clear error
// rather than a zero-valued, silently-wrong sample.
func TestGRPCTransport_Stats_RuntimeWithoutCapability(t *testing.T) {
	stream := newFakeSessionStream()
	tr := newGRPCTransport(newMux(stream))
	serveFakeAgent(t, stream, &fakeRuntime{})

	_, err := tr.Stats(context.Background(), "web-1")
	if err == nil {
		t.Fatal("Stats() error = nil, want the unsupported-capability error")
	}
	if !strings.Contains(err.Error(), "cannot report container stats") {
		t.Errorf("error = %v, want it to carry ErrStatsUnsupported's message", err)
	}
}

func TestLocal_Stats_DelegatesToWrappedRuntime(t *testing.T) {
	rt := &statsRuntime{stats: docker.ContainerStats{MemoryUsageBytes: 42}}
	local := NewLocal(rt)

	got, err := local.Stats(context.Background(), "web-1")
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if got.MemoryUsageBytes != 42 {
		t.Errorf("Stats() = %+v, want the wrapped runtime's own stats returned unchanged", got)
	}
}

func TestLocal_Stats_RuntimeWithoutCapability(t *testing.T) {
	local := NewLocal(&fakeRuntime{})

	if _, err := local.Stats(context.Background(), "web-1"); err != ErrStatsUnsupported { //nolint:errorlint // the sentinel is returned directly, not wrapped
		t.Errorf("error = %v, want ErrStatsUnsupported", err)
	}
}
