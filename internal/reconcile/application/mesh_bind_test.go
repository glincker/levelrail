package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMeshPaths struct {
	path meshpath.Path
	err  error
}

func (f *fakeMeshPaths) Path(context.Context, string) (meshpath.Path, error) { return f.path, f.err }

const meshHostPort = 18102

func meshDesired(bind string) *store.DesiredService {
	hp := meshHostPort
	return &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, HostPort: &hp, NodeID: "node-2", BindAddress: bind, Replicas: 1}
}

func usableMesh() *fakeMeshPaths {
	return &fakeMeshPaths{path: meshpath.Path{Usable: true, Address: "10.181.0.2"}}
}

func TestMeshBind_CreateBindsMeshAddress(t *testing.T) {
	tests := []struct {
		name  string
		bind  string
		paths *fakeMeshPaths
		want  string
	}{
		{name: "private default moves to mesh ip", bind: "", paths: usableMesh(), want: "10.181.0.2"},
		{name: "explicit private moves to mesh ip", bind: "private", paths: usableMesh(), want: "10.181.0.2"},
		{name: "public stays public", bind: "public", paths: usableMesh(), want: "0.0.0.0"},
		{name: "explicit ip stays", bind: "192.0.2.9", paths: usableMesh(), want: "192.0.2.9"},
		{name: "mesh down stays loopback", bind: "", paths: &fakeMeshPaths{path: meshpath.Path{Reason: "down"}}, want: "127.0.0.1"},
		{name: "resolver error stays loopback", bind: "", paths: &fakeMeshPaths{err: errors.New("boom")}, want: "127.0.0.1"},
		{name: "wildcard from resolver rejected", bind: "", paths: &fakeMeshPaths{path: meshpath.Path{Usable: true, Address: "0.0.0.0"}}, want: "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			rt.trackPorts = true
			c := New("web", &fakeStore{svc: meshDesired(tt.bind)}, rt, WithMeshPaths(tt.paths))
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := rt.lastCreateSpec.Ports[0].HostIP; got != tt.want {
				t.Fatalf("HostIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMeshBind_LocalAndUnplacedNeverRebound(t *testing.T) {
	for _, tt := range []struct {
		name   string
		nodeID string
		path   meshpath.Path
	}{
		{name: "no node id", nodeID: "", path: meshpath.Path{Usable: true, Address: "10.181.0.2"}},
		{name: "resolver says local", nodeID: "node-1", path: meshpath.Path{Local: true, Usable: true, Address: "10.181.0.1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			rt.trackPorts = true
			d := meshDesired("")
			d.NodeID = tt.nodeID
			c := New("web", &fakeStore{svc: d}, rt, WithMeshPaths(&fakeMeshPaths{path: tt.path}))
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if got := rt.lastCreateSpec.Ports[0].HostIP; got != "127.0.0.1" {
				t.Fatalf("HostIP = %q, want loopback", got)
			}
		})
	}
}

func TestMeshBind_OnlyMainPortMoves(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	st := &fakeStore{svc: meshDesired(""), streams: []store.AppStream{{ContainerPort: 5432, HostPort: 15432}}}
	c := New("web", st, rt, WithMeshPaths(usableMesh()))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	ports := rt.lastCreateSpec.Ports
	if len(ports) < 2 {
		t.Fatalf("ports = %+v, want main plus stream port", ports)
	}
	if ports[0].HostIP != "10.181.0.2" {
		t.Errorf("main HostIP = %q, want the mesh ip", ports[0].HostIP)
	}
	for _, p := range ports[1:] {
		if p.HostIP == "10.181.0.2" || p.HostIP == "0.0.0.0" {
			t.Errorf("extra port %+v must not be touched by the mesh bind", p)
		}
	}
}

func TestMeshBind_MeshComesUpAfterDeploy_RecreatesOnce(t *testing.T) {
	name := replicaContainerName("web", "img:v1", "", 0)
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	rt.seed(name, true)
	rt.containers[name].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: meshHostPort, HostIP: "127.0.0.1"}}
	paths := &fakeMeshPaths{path: meshpath.Path{Reason: "no handshake"}}
	c := New("web", &fakeStore{svc: meshDesired("")}, rt, WithMeshPaths(paths))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("mesh down pass error = %v", err)
	}
	if rt.createCalls != 0 {
		t.Fatalf("createCalls = %d, want the loopback container left alone while the mesh is down", rt.createCalls)
	}

	paths.path = meshpath.Path{Usable: true, Address: "10.181.0.2"}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("mesh up pass error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want exactly one recreate", rt.createCalls)
	}
	if got := rt.containers[name].Ports[0].HostIP; got != "10.181.0.2" {
		t.Fatalf("HostIP after recreate = %q, want the mesh ip", got)
	}

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("steady pass error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want no churn once bound to the mesh ip", rt.createCalls)
	}

	paths.path = meshpath.Path{Reason: "flap"}
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("flap pass error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, a mesh flap must not recreate the container", rt.createCalls)
	}
}

func TestMeshBind_HalfSuccess_RemoveFailsThenCreateFails(t *testing.T) {
	name := replicaContainerName("web", "img:v1", "", 0)
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	rt.seed(name, true)
	rt.containers[name].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: meshHostPort, HostIP: "127.0.0.1"}}
	c := New("web", &fakeStore{svc: meshDesired("")}, rt, WithMeshPaths(usableMesh()))

	rt.removeErr = errors.New("permission denied")
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("remove failure: error = nil, want it reported")
	}
	if cs := rt.containers[name]; cs == nil || !cs.Running {
		t.Fatal("remove failed, the serving container must be untouched")
	}

	rt.removeErr = nil
	rt.createErr = errors.New("daemon busy")
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("create failure: error = nil, want it reported")
	}

	rt.createErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("recovery pass error = %v, want the level-triggered retry to converge", err)
	}
	if got := rt.containers[name]; got == nil || !got.Running || got.Ports[0].HostIP != "10.181.0.2" {
		t.Fatalf("container = %+v, want running on the mesh ip", got)
	}
}
