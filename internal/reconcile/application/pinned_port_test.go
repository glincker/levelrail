package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const pinnedHostPort = 18101

func pinnedRuntime(t *testing.T, oldName string) *fakeRuntime {
	t.Helper()
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	rt.seed(oldName, true)
	rt.containers[oldName].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: pinnedHostPort}}
	return rt
}

func pinnedDesired(strategy string) *store.DesiredService {
	hp := pinnedHostPort
	return &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, HostPort: &hp, Strategy: strategy, Replicas: 1}
}

func TestPinnedPort_HandoffReplacesOldRelease(t *testing.T) {
	for _, strategy := range []string{"blue-green", "rolling"} {
		t.Run(strategy, func(t *testing.T) {
			v1 := replicaContainerName("web", "img:v1", "", 0)
			v2 := replicaContainerName("web", "img:v2", "", 0)
			rt := pinnedRuntime(t, v1)
			c := New("web", &fakeStore{svc: pinnedDesired(strategy)}, rt)

			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v, want the pinned port handed to the new release", err)
			}
			if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
				t.Fatalf("condition = %+v, want True", cond)
			}
			assertNames(t, "after handoff", rt.names(), []string{v2})
			if cs := rt.containers[v2]; !cs.Running || len(cs.Ports) != 1 || cs.Ports[0].HostPort != pinnedHostPort {
				t.Fatalf("new container = %+v, want running with pinned port %d published", cs, pinnedHostPort)
			}
		})
	}
}

func TestPinnedPort_FailedHandoffRestoresOldAndBacksOff(t *testing.T) {
	v1 := replicaContainerName("web", "img:v1", "", 0)
	rt := pinnedRuntime(t, v1)
	c := New("web", &fakeStore{svc: pinnedDesired("blue-green")}, rt, WithPinnedPortRetry(time.Hour))

	rt.createErr = errors.New("pull denied")
	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("first pass error = nil, want the create failure")
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != "CreateFailed" {
		t.Fatalf("first pass condition = %+v, want False/CreateFailed", cond)
	}
	if cs := rt.containers[v1]; cs == nil || !cs.Running {
		t.Fatalf("old release = %+v, want restored and running", cs)
	}

	rt.createErr = nil
	stopsBefore := rt.stopCalls
	res, err = c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("second pass error = nil, want the backoff error")
	}
	if cond := conditionOf(t, res); cond.Reason != reasonPinnedPortWait {
		t.Fatalf("second pass reason = %q, want %q", cond.Reason, reasonPinnedPortWait)
	}
	if rt.stopCalls != stopsBefore {
		t.Fatalf("stop calls during backoff = %d, want no new outage", rt.stopCalls-stopsBefore)
	}

	c.handoffFailed[replicaContainerName("web", "img:v2", "", 0)] = time.Now().Add(-time.Second)
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("pass after backoff error = %v, want convergence", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("pass after backoff condition = %+v, want True", cond)
	}
}

func TestPinnedPort_StartedWithoutPortIsRemoved(t *testing.T) {
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := pinnedRuntime(t, v1)
	rt.startNoPortsOnce = true
	c := New("web", &fakeStore{svc: pinnedDesired("blue-green")}, rt)

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the unpublished port reported")
	}
	if cond := conditionOf(t, res); cond.Reason != reasonPinnedPortUnbound {
		t.Fatalf("reason = %q, want %q", cond.Reason, reasonPinnedPortUnbound)
	}
	assertNames(t, "after failed handoff", rt.names(), []string{v1})
	if !rt.containers[v1].Running {
		t.Fatal("old release not running after the failed handoff")
	}
	if rt.containers[v2] != nil {
		t.Fatal("container without its pinned port was left behind")
	}
}

func TestPinnedPort_SteadyStateRecreatesContainerWithoutPort(t *testing.T) {
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	rt.seed(v2, true)
	c := New("web", &fakeStore{svc: pinnedDesired("blue-green")}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v, want the portless container recreated", err)
	}
	cs := rt.containers[v2]
	if cs == nil || !cs.Running || len(cs.Ports) != 1 || cs.Ports[0].HostPort != pinnedHostPort {
		t.Fatalf("container = %+v, want recreated with pinned port %d", cs, pinnedHostPort)
	}
}

func TestPinnedPort_UnpinnedKeepsBlueGreenOverlap(t *testing.T) {
	v1 := replicaContainerName("web", "img:v1", "", 0)
	v2 := replicaContainerName("web", "img:v2", "", 0)
	rt := newFakeRuntime(0)
	rt.seed(v1, true)
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green", Replicas: 1}
	c := New("web", &fakeStore{svc: desired}, rt)

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.stopCalls > 1 {
		t.Fatalf("stop calls = %d, want only the final retirement of the old release", rt.stopCalls)
	}
	create, remove := -1, -1
	for i, call := range rt.callOrder {
		switch call {
		case "create:" + v2:
			create = i
		case "remove:" + v1:
			remove = i
		}
	}
	if create < 0 || remove < 0 || create > remove {
		t.Fatalf("call order = %v, want the new release created before the old one is removed", rt.callOrder)
	}
}
