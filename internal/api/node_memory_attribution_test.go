package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestSplitHostMemory(t *testing.T) {
	const gb = 1 << 30
	tests := []struct {
		name                         string
		total, avail, svc, lr, other float64
		wantLR, wantOC, wantSys      float64
	}{
		{"remainder is system", 16 * gb, 8 * gb, 3 * gb, 1 * gb, 2 * gb, 1 * gb, 2 * gb, 2 * gb},
		{"no unmanaged", 16 * gb, 12 * gb, 1 * gb, 1 * gb, 0, 1 * gb, 0, 2 * gb},
		{"shares exceed used are clamped", 4 * gb, 3 * gb, 0.5 * gb, 1 * gb, 5 * gb, 0.5 * gb, 0, 0},
		{"available above total", 4 * gb, 5 * gb, 1 * gb, 1 * gb, 1 * gb, 0, 0, 0},
		{"negative inputs", 4 * gb, 2 * gb, -1, -1, -1, 0, 0, 2 * gb},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr, oc, sys := splitHostMemory(tt.total, tt.avail, tt.svc, tt.lr, tt.other)
			if lr != tt.wantLR || oc != tt.wantOC || sys != tt.wantSys {
				t.Fatalf("got lr=%v oc=%v sys=%v, want %v %v %v", lr, oc, sys, tt.wantLR, tt.wantOC, tt.wantSys)
			}
			if lr < 0 || oc < 0 || sys < 0 {
				t.Fatalf("negative segment: %v %v %v", lr, oc, sys)
			}
			used := max(tt.total-tt.avail, 0)
			svc := min(max(tt.svc, 0), used)
			if got := svc + lr + oc + sys; got != used {
				t.Fatalf("segments sum %v, want used %v", got, used)
			}
		})
	}
}

type fakeStats map[string]uint64

func (f fakeStats) Stats(_ context.Context, id string) (docker.ContainerStats, error) {
	return docker.ContainerStats{MemoryUsageBytes: f[id]}, nil
}

func TestUnmanagedUsage(t *testing.T) {
	desired := orphans.Desired{ContainerNames: map[string]bool{"web-abc": true}, ContainerNode: map[string]string{"web-abc": ""}}
	label := map[string]string{spec.InstanceLabelKey: "inst"}
	list := []docker.ContainerState{
		{ID: "a", Name: "web-abc", Running: true, Labels: label},
		{ID: "b", Name: "coolify-proxy", Running: true},
		{ID: "c", Name: "old-db", Running: true},
		{ID: "d", Name: "stopped", Running: false},
	}
	sum, top := unmanagedUsage(context.Background(), list, desired, fakeStats{"a": 900, "b": 300, "c": 700, "d": 500})
	if sum != 1000 {
		t.Fatalf("sum = %v, want 1000", sum)
	}
	if len(top) != 2 || top[0].Name != "old-db" || top[1].Name != "coolify-proxy" {
		t.Fatalf("top = %+v", top)
	}
}
