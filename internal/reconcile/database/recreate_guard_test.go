package database

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// podmanRuntime reports images the way Podman's compat API does: fully
// qualified with the docker.io registry and library namespace.
type podmanRuntime struct{ *fakeRuntime }

func (p podmanRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	st, err := p.fakeRuntime.InspectByName(ctx, name)
	if st != nil {
		cp := *st
		cp.Image = "docker.io/library/" + cp.Image
		return &cp, err
	}
	return st, err
}

// driftRuntime always reports an image that never matches the spec.
type driftRuntime struct{ *fakeRuntime }

func (d driftRuntime) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	st, err := d.fakeRuntime.InspectByName(ctx, name)
	if st != nil {
		cp := *st
		cp.Image = "mystery:1"
		return &cp, err
	}
	return st, err
}

func TestController_Reconcile_PodmanQualifiedImage_NoRecreate(t *testing.T) {
	rt := newFakeRuntime()
	rt.seed(containerName("main"), "redis:7", true)
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c := New("main", &fakeStore{db: desired}, podmanRuntime{rt})

	for i := 0; i < 5; i++ {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		if cond := conditionOf(t, res); cond.Reason != "AlreadyRunning" {
			t.Fatalf("pass %d: reason = %q, want AlreadyRunning", i, cond.Reason)
		}
	}
	if rt.createCalls != 0 || rt.removeCalls != 0 {
		t.Errorf("create=%d remove=%d, want no recreate for an equivalent image reference", rt.createCalls, rt.removeCalls)
	}
}

func TestSameImageRef(t *testing.T) {
	cases := []struct {
		observed, desired string
		want              bool
	}{
		{"docker.io/library/postgres:16", "postgres:16", true},
		{"docker.io/valkey/valkey:8", "valkey/valkey:8", true},
		{"docker.io/library/postgres:15", "postgres:16", false},
		{"ghcr.io/library/postgres:16", "postgres:16", false},
		{"postgres:16", "postgres:16", true},
	}
	for _, tc := range cases {
		if got := sameImageRef(tc.observed, tc.desired); got != tc.want {
			t.Errorf("sameImageRef(%q, %q) = %v, want %v", tc.observed, tc.desired, got, tc.want)
		}
	}
}

func TestController_Reconcile_RecreateLoop_BacksOffAndSurfacesCondition(t *testing.T) {
	t.Setenv(EnvRecreateLimit, "3")
	t.Setenv(EnvRecreateWindow, "10m")
	t.Setenv(EnvRecreateBackoffBase, "30s")
	t.Setenv(EnvRecreateBackoffMax, "5m")

	rt := newFakeRuntime()
	rt.seed(containerName("main"), "redis:6", true)
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c := New("main", &fakeStore{db: desired}, driftRuntime{rt})
	c.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		res, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
		if cond := conditionOf(t, res); cond.Reason != "Deployed" {
			t.Fatalf("pass %d: reason = %q, want Deployed while under the limit", i, cond.Reason)
		}
		now = now.Add(2 * time.Second)
	}
	if rt.createCalls != 3 {
		t.Fatalf("createCalls = %d, want 3 before the guard trips", rt.createCalls)
	}

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("guarded pass returned error %v, want a condition only", err)
	}
	cond := conditionOf(t, res)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != reasonRecreateLoop || cond.Message == "" {
		t.Fatalf("condition = %+v, want False/RecreateLoop with a message", cond)
	}
	if res.RequeueAfter <= 0 || res.RequeueAfter > 30*time.Second {
		t.Errorf("RequeueAfter = %s, want within the 30s first backoff", res.RequeueAfter)
	}
	if rt.createCalls != 3 {
		t.Errorf("createCalls = %d, want no recreate while backing off", rt.createCalls)
	}

	// After the first delay one more recreate is allowed, then the wait doubles.
	now = now.Add(31 * time.Second)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != 4 {
		t.Fatalf("createCalls = %d, want 4 after the first backoff elapsed", rt.createCalls)
	}
	now = now.Add(31 * time.Second)
	res, _ = c.Reconcile(context.Background())
	if cond := conditionOf(t, res); cond.Reason != reasonRecreateLoop {
		t.Fatalf("reason = %q, want the doubled 60s delay to still hold at +31s", cond.Reason)
	}
	now = now.Add(31 * time.Second)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.createCalls != 5 {
		t.Errorf("createCalls = %d, want 5 once the doubled delay elapsed", rt.createCalls)
	}
}

func TestRecreateGuard_WindowExpiryResets(t *testing.T) {
	t.Setenv(EnvRecreateLimit, "2")
	t.Setenv(EnvRecreateWindow, "1m")
	var g recreateGuard
	now := time.Now()
	for i := 0; i < 2; i++ {
		if err := g.allow(now); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.allow(now); err == nil {
		t.Fatal("third recreate inside the window should be held")
	}
	if err := g.allow(now.Add(2 * time.Minute)); err != nil {
		t.Fatalf("after the window expired: %v", err)
	}
}
