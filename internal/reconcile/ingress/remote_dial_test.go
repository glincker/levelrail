package ingress

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePaths struct{ path meshpath.Path }

func (f *fakePaths) Path(context.Context, string) (meshpath.Path, error) { return f.path, nil }

func remoteSvc() store.DesiredService {
	return store.DesiredService{Name: "nodes-web", Image: "nginx:1", Port: 80, NodeID: "node-2", Domains: []string{"nodes-web.example.com"}}
}

func crossNodeCond(res reconcile.Result) *reconcile.Condition {
	for i := range res.Conditions {
		if res.Conditions[i].Type == "CrossNodeIngress" {
			return &res.Conditions[i]
		}
	}
	return nil
}

func TestController_RemoteService_RoutesOverMesh(t *testing.T) {
	svc := remoteSvc()
	name := application.ContainerName(svc.Name, svc.Image, "")
	tests := []struct {
		name     string
		hostIP   string
		path     meshpath.Path
		wantDial string
		wantWhy  string
	}{
		{name: "mesh bound", hostIP: "10.181.0.2", path: meshpath.Path{Usable: true, Address: "10.181.0.2"}, wantDial: "10.181.0.2:32768"},
		{name: "public bind reachable via mesh", hostIP: "0.0.0.0", path: meshpath.Path{Usable: true, Address: "10.181.0.2"}, wantDial: "10.181.0.2:32768"},
		{name: "loopback awaits recreate", hostIP: "127.0.0.1", path: meshpath.Path{Usable: true, Address: "10.181.0.2"}, wantWhy: "loopback"},
		{name: "foreign interface", hostIP: "192.0.2.9", path: meshpath.Path{Usable: true, Address: "10.181.0.2"}, wantWhy: "192.0.2.9"},
		{name: "mesh down", hostIP: "127.0.0.1", path: meshpath.Path{Reason: "no recent WireGuard handshake"}, wantWhy: "handshake"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := newFakeRuntime()
			remote.containers[name] = &docker.ContainerState{Name: name, Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 32768, HostIP: tt.hostIP}}}
			applier := &fakeApplier{}
			c := New(&fakeStore{services: []store.DesiredService{svc}}, newFakeRuntime(), applier,
				WithLogger(discardLogger()), WithLocalNodeID("node-1"),
				WithNodeUpstreams(fakeNodes{rt: remote, host: "10.181.0.2"}), WithMeshPaths(&fakePaths{path: tt.path}))
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cond := crossNodeCond(res)
			if tt.wantDial != "" {
				if cond != nil {
					t.Fatalf("CrossNodeIngress = %+v, want it cleared", cond)
				}
				if rp := proxyHandler(t, applier); rp.Upstreams[0].Dial != tt.wantDial {
					t.Fatalf("dial = %q, want %q", rp.Upstreams[0].Dial, tt.wantDial)
				}
				return
			}
			if len(applier.routes(t)) != 0 {
				t.Fatal("want no route while the mesh path is unusable")
			}
			if cond == nil || cond.Reason != "NoMeshIngressPath" || !strings.Contains(cond.Message, tt.wantWhy) {
				t.Fatalf("CrossNodeIngress = %+v, want NoMeshIngressPath mentioning %q", cond, tt.wantWhy)
			}
		})
	}
}

func TestController_RemoteService_ConditionClearsWhenMeshComesUp(t *testing.T) {
	svc := remoteSvc()
	name := application.ContainerName(svc.Name, svc.Image, "")
	remote := newFakeRuntime()
	remote.containers[name] = &docker.ContainerState{Name: name, Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 32768, HostIP: "10.181.0.2"}}}
	paths := &fakePaths{path: meshpath.Path{Reason: "node has not joined the mesh yet"}}
	applier := &fakeApplier{}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, newFakeRuntime(), applier,
		WithLogger(discardLogger()), WithLocalNodeID("node-1"),
		WithNodeUpstreams(fakeNodes{rt: remote, host: "10.181.0.2"}), WithMeshPaths(paths))

	res, err := c.Reconcile(context.Background())
	if err != nil || crossNodeCond(res) == nil {
		t.Fatalf("mesh down: err=%v cond=%+v, want the condition set", err, crossNodeCond(res))
	}
	paths.path = meshpath.Path{Usable: true, Address: "10.181.0.2"}
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := crossNodeCond(res); cond != nil {
		t.Fatalf("mesh up: CrossNodeIngress = %+v, want cleared", cond)
	}
	if rp := proxyHandler(t, applier); rp.Upstreams[0].Dial != "10.181.0.2:32768" {
		t.Fatalf("dial = %q", rp.Upstreams[0].Dial)
	}
}

func TestController_RemoteService_NotReadyStaysQuiet(t *testing.T) {
	svc := remoteSvc()
	remote := newFakeRuntime()
	st := &fakeStore{services: []store.DesiredService{svc}}
	c := New(st, newFakeRuntime(), &fakeApplier{}, WithLogger(discardLogger()), WithLocalNodeID("node-1"),
		WithNodeUpstreams(fakeNodes{rt: remote, host: "10.181.0.2"}),
		WithMeshPaths(&fakePaths{path: meshpath.Path{Usable: true, Address: "10.181.0.2"}}))
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cond := crossNodeCond(res); cond != nil {
		t.Fatalf("no container yet is a mid-deploy skip, got %+v", cond)
	}
}
