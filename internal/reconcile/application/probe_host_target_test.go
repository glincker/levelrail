package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/probe"
)

type hostProbeRuntime struct {
	docker.Runtime
	can bool
}

func (hostProbeRuntime) CheckRemote(context.Context, string, probe.Config) error { return nil }
func (h hostProbeRuntime) ProbesOnHost() bool                                    { return h.can }

func TestProbeTarget_NoMeshRemoteProbesOnAgent(t *testing.T) {
	tcp := store.ServiceProbe{TCP: true}
	state := &docker.ContainerState{ID: "c1", Name: "web", Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 32769, HostIP: "127.0.0.1"}}}
	noMesh := &fakeMeshPaths{path: meshpath.Path{NoMesh: true, Reason: "not joined"}}
	tests := []struct {
		name       string
		paths      *fakeMeshPaths
		rt         docker.Runtime
		node       string
		wantRemote bool
	}{
		{name: "no mesh remote with capable agent", paths: noMesh, rt: hostProbeRuntime{Runtime: newFakeRuntime(0), can: true}, node: "mac", wantRemote: true},
		{name: "no mesh resolver unset", paths: nil, rt: hostProbeRuntime{Runtime: newFakeRuntime(0), can: true}, node: "mac", wantRemote: true},
		{name: "older agent keeps loopback", paths: noMesh, rt: hostProbeRuntime{Runtime: newFakeRuntime(0), can: false}, node: "mac"},
		{name: "plain runtime keeps loopback", paths: noMesh, rt: newFakeRuntime(0), node: "mac"},
		{name: "local node never remote", paths: &fakeMeshPaths{path: meshpath.Path{Local: true}}, rt: hostProbeRuntime{Runtime: newFakeRuntime(0), can: true}, node: "node-1"},
		{name: "mesh node dials mesh address", paths: usableMesh(), rt: hostProbeRuntime{Runtime: newFakeRuntime(0), can: true}, node: "node-2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []Option{}
			if tt.paths != nil {
				opts = append(opts, WithMeshPaths(tt.paths))
			}
			c := New("web", &fakeStore{}, tt.rt, opts...)
			st := state
			if tt.name == "mesh node dials mesh address" {
				st = &docker.ContainerState{ID: "c1", Name: "web", Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 32769, HostIP: "0.0.0.0"}}}
			}
			got, err := c.probeTarget(context.Background(), st, tcp, 80, tt.node)
			if err != nil {
				t.Fatal(err)
			}
			if (got.Remote != nil) != tt.wantRemote {
				t.Errorf("remote = %v, want %v", got.Remote != nil, tt.wantRemote)
			}
			if tt.node == "node-2" && got.Addr != "10.181.0.2:32769" {
				t.Errorf("mesh addr = %q", got.Addr)
			}
		})
	}
}
