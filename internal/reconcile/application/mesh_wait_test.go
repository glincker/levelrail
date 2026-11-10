package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

func readyCond(t *testing.T, r reconcile.Result) reconcile.Condition {
	t.Helper()
	for _, c := range r.Conditions {
		if c.Type == reconcile.ConditionTypeReady {
			return c
		}
	}
	t.Fatalf("no Ready condition in %+v", r.Conditions)
	return reconcile.Condition{}
}

func TestMeshWait_WaitsThenStartsWithoutRedeploy(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	paths := &fakeMeshPaths{path: meshpath.Path{Reason: "no recent WireGuard handshake"}}
	c := New("web", &fakeStore{svc: meshDesired("")}, rt, WithMeshPaths(paths))

	for i := range 3 {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: waiting must not be an error, got %v", i, err)
		}
		cond := readyCond(t, res)
		if cond.Reason != ReasonWaitingForMesh || cond.Status != reconcile.ConditionUnknown {
			t.Fatalf("pass %d: condition = %+v, want Unknown %s", i, cond, ReasonWaitingForMesh)
		}
		if res.RequeueAfter <= 0 || !res.KeyOnly {
			t.Fatalf("pass %d: want a keyed requeue, got %+v", i, res)
		}
		if rt.createCalls != 0 {
			t.Fatalf("pass %d: createCalls = %d, nothing may start while the mesh is down", i, rt.createCalls)
		}
	}

	paths.path = meshpath.Path{Usable: true, Address: "10.181.0.2"}
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("mesh up pass error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Fatalf("createCalls = %d, want the service started once the mesh is healthy", rt.createCalls)
	}
	if got := rt.lastCreateSpec.Ports[0].HostIP; got != "10.181.0.2" {
		t.Fatalf("HostIP = %q, want the mesh ip", got)
	}
	if readyCond(t, res).Reason == ReasonWaitingForMesh {
		t.Fatal("still waiting after the mesh became healthy")
	}
}

func TestMeshWait_BackoffBounded(t *testing.T) {
	limit := 9 * time.Second
	c := New("web", &fakeStore{svc: meshDesired("")}, newFakeRuntime(0),
		WithMeshPaths(&fakeMeshPaths{path: meshpath.Path{Reason: "down"}}),
		WithMeshRetry(func(context.Context) time.Duration { return limit }))
	var got []time.Duration
	for range 8 {
		res, _ := c.Reconcile(context.Background())
		got = append(got, res.RequeueAfter)
	}
	if got[0] != meshRetryFloor {
		t.Errorf("first delay = %s, want %s", got[0], meshRetryFloor)
	}
	for i, d := range got {
		if d <= 0 || d > limit {
			t.Fatalf("delay %d = %s, want within (0, %s]", i, d, limit)
		}
		if i > 0 && d < got[i-1] {
			t.Fatalf("delays must not shrink while waiting: %v", got)
		}
	}
	if got[len(got)-1] != limit {
		t.Errorf("last delay = %s, want the ceiling %s", got[len(got)-1], limit)
	}
}

func TestMeshWait_MeshDisabledUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name  string
		paths meshpath.Resolver
		node  string
	}{
		{name: "no resolver", paths: nil, node: "node-2"},
		{name: "mesh off", paths: &fakeMeshPaths{path: meshpath.Path{NoMesh: true, Reason: "mesh networking is off"}}, node: "node-2"},
		{name: "local node", paths: &fakeMeshPaths{path: meshpath.Path{Local: true}}, node: "node-1"},
		{name: "unplaced", paths: &fakeMeshPaths{path: meshpath.Path{Reason: "down"}}, node: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			d := meshDesired("")
			d.NodeID = tt.node
			d.HostPort = nil
			var opts []Option
			if tt.paths != nil {
				opts = append(opts, WithMeshPaths(tt.paths))
			}
			res, err := New("web", &fakeStore{svc: d}, rt, opts...).Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if rt.createCalls != 1 || readyCond(t, res).Reason == ReasonWaitingForMesh {
				t.Fatalf("createCalls = %d, result %+v: want the old start path", rt.createCalls, res)
			}
		})
	}
}

func TestMeshWait_RunningReplicaNotBlocked(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.seed(replicaContainerName("web", "img:v1", "", 0), true)
	d := meshDesired("")
	d.HostPort = nil
	c := New("web", &fakeStore{svc: d}, rt, WithMeshPaths(&fakeMeshPaths{path: meshpath.Path{Reason: "flap"}}))
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if readyCond(t, res).Reason == ReasonWaitingForMesh {
		t.Fatal("a mesh flap must not park a serving replica")
	}
}

func TestMeshWait_NodeRemovedDoesNotLeakRequeue(t *testing.T) {
	paths := &fakeMeshPaths{path: meshpath.Path{Reason: "down"}}
	c := New("web", &fakeStore{svc: meshDesired("")}, newFakeRuntime(0), WithMeshPaths(paths))
	if res, _ := c.Reconcile(context.Background()); readyCond(t, res).Reason != ReasonWaitingForMesh {
		t.Fatalf("want a wait first, got %+v", res)
	}
	paths.err = errors.New("meshpath: load node: not found")
	res, _ := c.Reconcile(context.Background())
	if readyCond(t, res).Reason == ReasonWaitingForMesh || (res.KeyOnly && res.RequeueAfter > 0) {
		t.Fatalf("removed node still requeues the mesh wait: %+v", res)
	}
	if c.meshWaitStreak != 0 {
		t.Fatalf("wait streak = %d, want cleared", c.meshWaitStreak)
	}
}
