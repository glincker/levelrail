package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestProbeTarget_RemoteNodeDialsMeshAddress(t *testing.T) {
	tcp := store.ServiceProbe{TCP: true}
	state := func(hostIP string) *docker.ContainerState {
		return &docker.ContainerState{ID: "c1", Name: "web", Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 18102, HostIP: hostIP}}}
	}
	tests := []struct {
		name     string
		paths    *fakeMeshPaths
		node     string
		hostIP   string
		wantAddr string
		wantErr  bool
	}{
		{name: "remote on mesh ip", paths: usableMesh(), node: "node-2", hostIP: "10.181.0.2", wantAddr: "10.181.0.2:18102"},
		{name: "remote wildcard bind uses mesh ip", paths: usableMesh(), node: "node-2", hostIP: "0.0.0.0", wantAddr: "10.181.0.2:18102"},
		{name: "remote loopback bind is not dialable", paths: usableMesh(), node: "node-2", hostIP: "127.0.0.1", wantErr: true},
		{name: "remote with mesh down errors", paths: &fakeMeshPaths{path: meshpath.Path{Reason: "down"}}, node: "node-2", hostIP: "10.181.0.2", wantErr: true},
		{name: "mesh off falls back to loopback", paths: &fakeMeshPaths{path: meshpath.Path{NoMesh: true, Reason: "off"}}, node: "node-2", hostIP: "127.0.0.1", wantAddr: "127.0.0.1:18102"},
		{name: "node without mesh address falls back to loopback", paths: &fakeMeshPaths{path: meshpath.Path{NoMesh: true, Reason: "not joined"}}, node: "node-2", hostIP: "10.0.0.5", wantAddr: "127.0.0.1:18102"},
		{name: "resolver error", paths: &fakeMeshPaths{err: errors.New("boom")}, node: "node-2", hostIP: "10.181.0.2", wantErr: true},
		{name: "local node stays loopback", paths: &fakeMeshPaths{path: meshpath.Path{Local: true}}, node: "node-1", hostIP: "127.0.0.1", wantAddr: "127.0.0.1:18102"},
		{name: "no node id stays loopback", paths: usableMesh(), node: "", hostIP: "127.0.0.1", wantAddr: "127.0.0.1:18102"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("web", &fakeStore{}, newFakeRuntime(0), WithMeshPaths(tt.paths))
			got, err := c.probeTarget(context.Background(), state(tt.hostIP), tcp, tt.node)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got.Addr != tt.wantAddr {
				t.Errorf("addr = %q, want %q", got.Addr, tt.wantAddr)
			}
		})
	}
}

// A mesh outage makes probes unreachable, which is not evidence the app is
// unhealthy: nothing may be stopped, restarted or flipped unhealthy.
func TestRemoteProbeUnreachable_NeverRestartsOrFlipsCondition(t *testing.T) {
	down := &fakeMeshPaths{path: meshpath.Path{Reason: "mesh down"}}
	tests := []struct {
		name   string
		health *store.ServiceHealth
	}{
		{name: "liveness", health: &store.ServiceHealth{Liveness: &store.ServiceProbe{Path: "/livez", Interval: livenessProbeInterval, Failures: 1}}},
		{name: "readiness recheck", health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/ready"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(18102)
			desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, NodeID: "node-2", Health: tt.health}
			rt.seed(ContainerName("web", desired.Image, ""), true)
			c := New("web", &fakeStore{svc: desired}, rt, WithMeshPaths(down))
			for i := 0; i < 3; i++ {
				result, _ := c.Reconcile(context.Background())
				if cond := conditionOf(t, result); cond.Status != "True" {
					t.Fatalf("pass %d condition = %+v, want Status=True", i, cond)
				}
				time.Sleep(livenessProbeInterval + time.Millisecond)
			}
			if rt.stopCalls != 0 || rt.removeCalls != 0 || rt.startCalls != 0 {
				t.Errorf("stop/remove/start = %d/%d/%d, want none", rt.stopCalls, rt.removeCalls, rt.startCalls)
			}
		})
	}
}
