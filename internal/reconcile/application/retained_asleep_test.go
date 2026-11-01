package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// asleepGuard mirrors the store: the sleeping mark persists independently of Suspended.
type asleepGuard struct {
	retainedYes
	sleeping bool
}

func (g *asleepGuard) Asleep(context.Context, string) bool { return g.sleeping }

func passes(t *testing.T, c *Controller, n int) string {
	t.Helper()
	reason := ""
	for i := 0; i < n; i++ {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		reason = res.Conditions[0].Reason
	}
	return reason
}

// A deploy copy that is unsuspended by a stray start but still marked sleeping
// creates nothing across reconcile passes, and reads as stopped.
func TestRetainedAsleepCopyCreatesNoContainer(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{Name: "web-aaaaaaa", Image: "img:v1", Port: 80, Replicas: 1}
	g := &asleepGuard{sleeping: true}
	c := New("web-aaaaaaa", &fakeStore{svc: desired}, rt, WithRetainedDeploys(g))
	if got := passes(t, c, 3); got != "Suspended" {
		t.Fatalf("reason = %q, want Suspended", got)
	}
	if len(rt.containers) != 0 {
		t.Fatalf("created %d containers for a sleeping copy", len(rt.containers))
	}
}

// Stop then start of a copy: the start alone (no wake) leaves it down; a wake
// (sleeping cleared first, then unsuspended) brings it up.
func TestRetainedStopThenStartStaysAsleepUntilWoken(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{Name: "web-aaaaaaa", Image: "img:v1", Port: 80, Replicas: 1, Suspended: true}
	g := &asleepGuard{sleeping: true}
	c := New("web-aaaaaaa", &fakeStore{svc: desired}, rt, WithRetainedDeploys(g), WithReadyBudget(1e9))
	if got := passes(t, c, 1); got != reasonRetainedDeployStopped {
		t.Fatalf("stopped reason = %q", got)
	}
	desired.Suspended = false
	if got := passes(t, c, 2); got != "Suspended" || runningCount(rt) != 0 {
		t.Fatalf("after start: reason %q, running %d", got, runningCount(rt))
	}
	g.sleeping = false
	passes(t, c, 1)
	if runningCount(rt) != 1 {
		t.Fatalf("after wake: running %d, want 1", runningCount(rt))
	}
}
