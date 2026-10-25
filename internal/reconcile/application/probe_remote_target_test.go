package application

import (
	"context"
	"errors"
	"testing"

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
