package database

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeEndpoints map[string]meshpath.Path

func (f fakeEndpoints) Endpoint(_ context.Context, nodeID string) (meshpath.Path, error) {
	return f[nodeID], nil
}

type fakeMeshPorts map[string]int

func (f fakeMeshPorts) GetDatabaseMeshPort(_ context.Context, name string) (int, error) {
	return f[name], nil
}

func (f fakeMeshPorts) GetDatabasePoolerMeshPort(_ context.Context, name string) (int, error) {
	return f[name+"-pooler"], nil
}

func TestMeshBinding(t *testing.T) {
	up := fakeEndpoints{"n2": {Usable: true, Address: "10.181.0.2"}}
	down := fakeEndpoints{"n2": {Reason: "no handshake"}}
	wildcard := fakeEndpoints{"n2": {Usable: true, Address: "0.0.0.0"}}
	bound := &docker.ContainerState{Ports: []docker.PortBinding{{ContainerPort: 5432, HostPort: 21000, HostIP: "10.181.0.2", Protocol: "tcp"}}}
	tests := []struct {
		name  string
		ep    fakeEndpoints
		ports fakeMeshPorts
		state *docker.ContainerState
		want  *docker.PortBinding
	}{
		{name: "no cross-node consumer, no binding", ep: up, ports: fakeMeshPorts{}},
		{name: "port allocated and path up", ep: up, ports: fakeMeshPorts{"main": 21000},
			want: &docker.PortBinding{ContainerPort: 5432, HostPort: 21000, HostIP: "10.181.0.2"}},
		{name: "path down keeps the observed binding", ep: down, ports: fakeMeshPorts{"main": 21000}, state: bound,
			want: &docker.PortBinding{ContainerPort: 5432, HostPort: 21000, HostIP: "10.181.0.2", Protocol: "tcp"}},
		{name: "path down and nothing bound yet", ep: down, ports: fakeMeshPorts{"main": 21000}},
		{name: "never a wildcard bind", ep: wildcard, ports: fakeMeshPorts{"main": 21000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("main", nil, nil, WithMeshEndpoint(tt.ep, tt.ports))
			got, ok := c.meshBinding(context.Background(), &store.DesiredDatabase{Name: "main", NodeID: "n2"}, tt.state, 5432)
			if (tt.want != nil) != ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tt.want != nil, got)
			}
			if tt.want != nil && got != *tt.want {
				t.Fatalf("binding = %+v, want %+v", got, *tt.want)
			}
		})
	}
}

func TestPoolerMeshBinding(t *testing.T) {
	up := fakeEndpoints{"n2": {Usable: true, Address: "10.181.0.2"}}
	c := &Controller{dbName: "main", meshEndpoints: up, meshPorts: fakeMeshPorts{"main-pooler": 21005}}
	on := &store.DesiredDatabase{Engine: store.EnginePostgres, NodeID: "n2", PoolerEnabled: true}
	b, ok := c.poolerMeshBinding(context.Background(), on, nil)
	if !ok || b.ContainerPort != PoolerPort || b.HostPort != 21005 || b.HostIP != "10.181.0.2" {
		t.Fatalf("binding = %+v, %v", b, ok)
	}
	off := &store.DesiredDatabase{Engine: store.EnginePostgres, NodeID: "n2"}
	if _, ok := c.poolerMeshBinding(context.Background(), off, nil); ok {
		t.Fatal("binding published without an enabled pooler")
	}
}
