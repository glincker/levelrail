package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestFailedReadinessCandidateIsStoppedNotLeftUp(t *testing.T) {
	srv := neverHealthy()
	defer srv.Close()

	rt := newFakeRuntime(serverPort(t, srv))
	desired := &store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}},
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(150*time.Millisecond))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want a readiness failure")
	}
	name := replicaContainerName("web", "img:v1", "", 0)
	cs := rt.containers[name]
	if cs == nil {
		t.Fatal("failed candidate was removed, want it kept stopped for log retrieval")
	}
	if cs.Running {
		t.Fatal("failed candidate still running after readiness failure")
	}
}
