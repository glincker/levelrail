package agent

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/spec"
)

type fakeContainerStats map[string]uint64

func (f fakeContainerStats) Stats(_ context.Context, id string) (docker.ContainerStats, error) {
	return docker.ContainerStats{MemoryUsageBytes: f[id]}, nil
}

func TestUnmanagedContainerUsage(t *testing.T) {
	managed := map[string]string{spec.InstanceLabelKey: "inst"}
	tests := []struct {
		name    string
		list    []docker.ContainerState
		wantSum uint64
		want    []string
	}{
		{"labelled and stopped are skipped", []docker.ContainerState{
			{ID: "a", Name: "web", Running: true, Labels: managed},
			{ID: "b", Name: "proxy", Running: true},
			{ID: "c", Name: "off", Running: false},
		}, 300, []string{"proxy"}},
		{"sorted and capped at top N", []docker.ContainerState{
			{ID: "1", Name: "c1", Running: true}, {ID: "2", Name: "c2", Running: true},
			{ID: "3", Name: "c3", Running: true}, {ID: "4", Name: "c4", Running: true},
			{ID: "5", Name: "c5", Running: true}, {ID: "6", Name: "c6", Running: true},
		}, 21, []string{"c6", "c5", "c4", "c3", "c2"}},
		{"nothing unmanaged", nil, 0, nil},
	}
	stats := fakeContainerStats{"a": 900, "b": 300, "c": 500, "1": 1, "2": 2, "3": 3, "4": 4, "5": 5, "6": 6}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum, top := unmanagedContainerUsage(context.Background(), tt.list, stats)
			if sum != tt.wantSum {
				t.Fatalf("sum = %d, want %d", sum, tt.wantSum)
			}
			if len(top) != len(tt.want) {
				t.Fatalf("top = %v, want names %v", top, tt.want)
			}
			for i, n := range tt.want {
				if top[i].Name != n {
					t.Fatalf("top[%d] = %q, want %q", i, top[i].Name, n)
				}
			}
		})
	}
}

func TestHostReportPBRoundTrip(t *testing.T) {
	in := HostReport{
		MemoryTotalBytes: 8 << 30, MemoryAvailableBytes: 3 << 30, AgentRSSBytes: 40 << 20,
		OtherContainersMemoryBytes: 512 << 20,
		OtherContainers:            []UnmanagedContainer{{Name: "proxy", MemoryBytes: 512 << 20}},
	}
	out := hostReportFromPB(hostReportToPB(in))
	if out.MemoryTotalBytes != in.MemoryTotalBytes || out.AgentRSSBytes != in.AgentRSSBytes ||
		out.OtherContainersMemoryBytes != in.OtherContainersMemoryBytes ||
		len(out.OtherContainers) != 1 || out.OtherContainers[0] != in.OtherContainers[0] {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}

func TestHostReportStoreStaleness(t *testing.T) {
	now := time.Unix(1000, 0)
	s := NewHostReportStore()
	s.now = func() time.Time { return now }
	if _, ok := s.Get("n1"); ok {
		t.Fatal("Get before any report = ok, want false")
	}
	s.SetNodeHostReport("n1", HostReport{AgentRSSBytes: 7})
	if r, ok := s.Get("n1"); !ok || r.AgentRSSBytes != 7 {
		t.Fatalf("fresh Get = %+v, %v", r, ok)
	}
	now = now.Add(hostReportMaxAge + time.Second)
	if _, ok := s.Get("n1"); ok {
		t.Fatal("stale Get = ok, want false")
	}
}
