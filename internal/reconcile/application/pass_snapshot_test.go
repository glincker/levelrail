package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// batchRuntime is fakeRuntime plus docker.BatchInspector.
type batchRuntime struct {
	*fakeRuntime
	batches int
}

func (r *batchRuntime) InspectManyByName(_ context.Context, names []string) (map[string]*docker.ContainerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches++
	out := make(map[string]*docker.ContainerState, len(names))
	for _, n := range names {
		out[n] = nil
		if cs, ok := r.containers[n]; ok {
			cp := *cs
			out[n] = &cp
		}
	}
	return out, nil
}

func runningDesired(name string) *store.DesiredService {
	return &store.DesiredService{Name: name, Image: "img:v1", Port: 80}
}

func suspendedDesired(name string) *store.DesiredService {
	return &store.DesiredService{Name: name, Image: "img:v1", Port: 80, Suspended: true}
}

func TestPassSnapshot_SuspendedApps_ShareOneListing(t *testing.T) {
	rt := newFakeRuntime(0)
	snap := NewPassSnapshot(time.Minute)
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("app%02d", i)
		c := New(name, &fakeStore{svc: suspendedDesired(name)}, rt, WithPassSnapshot(snap, "local"))
		result, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("%s: Reconcile() error = %v", name, err)
		}
		if cond := conditionOf(t, result); cond.Reason != "Suspended" {
			t.Fatalf("%s: reason = %q, want Suspended", name, cond.Reason)
		}
	}
	if rt.listCalls != 1 {
		t.Errorf("ListByPrefix calls for 20 stopped apps = %d, want 1 shared listing", rt.listCalls)
	}
}

func TestPassSnapshot_SuspendedApp_StillRemovesItsContainersAndOnlyThose(t *testing.T) {
	rt := newFakeRuntime(0)
	mine := ContainerName("web", "img:v1", "")
	sibling := ContainerName("web-worker", "img:v1", "")
	rt.seed(mine, true)
	rt.seed(sibling, true)

	snap := NewPassSnapshot(time.Minute)
	c := New("web", &fakeStore{svc: suspendedDesired("web")}, rt, WithPassSnapshot(snap, "local"))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	names := rt.names()
	if len(names) != 1 || names[0] != sibling {
		t.Errorf("containers after suspending web = %v, want only %s left", names, sibling)
	}
}

func TestPassSnapshot_StoppedAppStaysStoppedAcrossPasses(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.seed(ContainerName("web", "img:v1", ""), true)
	st := &fakeStore{svc: suspendedDesired("web")}

	for pass := 1; pass <= 3; pass++ {
		snap := NewPassSnapshot(time.Minute)
		c := New("web", st, rt, WithPassSnapshot(snap, "local"))
		result, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("pass %d: Reconcile() error = %v", pass, err)
		}
		if cond := conditionOf(t, result); cond.Status != reconcile.ConditionUnknown || cond.Reason != "Suspended" {
			t.Errorf("pass %d: condition = %+v, want Unknown/Suspended", pass, cond)
		}
		if names := rt.names(); len(names) != 0 {
			t.Errorf("pass %d: containers = %v, want none", pass, names)
		}
	}
	if rt.createCalls != 0 || rt.startCalls != 0 {
		t.Errorf("a stopped app must never be created or started, got create=%d start=%d", rt.createCalls, rt.startCalls)
	}
}

func TestPassSnapshot_NextPassCatchesContainerStartedWithoutAnEvent(t *testing.T) {
	rt := newFakeRuntime(0)
	st := &fakeStore{svc: suspendedDesired("web")}

	first := New("web", st, rt, WithPassSnapshot(NewPassSnapshot(time.Minute), "local"))
	if _, err := first.Reconcile(context.Background()); err != nil {
		t.Fatalf("first pass error = %v", err)
	}

	rt.seed(ContainerName("web", "img:v1", ""), true)

	second := New("web", st, rt, WithPassSnapshot(NewPassSnapshot(time.Minute), "local"))
	if _, err := second.Reconcile(context.Background()); err != nil {
		t.Fatalf("second pass error = %v", err)
	}
	if names := rt.names(); len(names) != 0 {
		t.Errorf("containers after the resync pass = %v, want the stray container removed", names)
	}
}

func TestPassSnapshot_DriftedAppIsStillCorrected(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	oldTarget := ContainerName("web", "img:v1", "")
	rt.seed(oldTarget, true)

	desired := &store.DesiredService{
		Name: "web", Image: "img:v2", Port: 80,
		Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz", Interval: 10 * time.Millisecond, Timeout: 200 * time.Millisecond}},
	}
	snap := NewPassSnapshot(time.Minute)
	c := New("web", &fakeStore{svc: desired}, rt, WithReadyBudget(2*time.Second), WithPassSnapshot(snap, "local"))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	newTarget := ContainerName("web", "img:v2", "")
	names := rt.names()
	if len(names) != 1 || names[0] != newTarget {
		t.Errorf("containers after drift correction = %v, want exactly [%s]", names, newTarget)
	}
}

func TestPassSnapshot_CrashedContainerIsRestartedWhileSnapshotIsWarm(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	target := ContainerName("web", desired.Image, "")
	rt.seed(target, false)

	snap := NewPassSnapshot(time.Minute)
	if _, err := snap.ListByPrefix(context.Background(), "local", rt, ""); err != nil {
		t.Fatalf("warming the snapshot: %v", err)
	}
	c := New("web", &fakeStore{svc: desired}, rt, WithPassSnapshot(snap, "local"))
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want True after the restart", cond)
	}
	if cs, _ := rt.InspectByName(context.Background(), target); cs == nil || !cs.Running {
		t.Errorf("crashed container was not restarted: %+v", cs)
	}
}

func TestPassSnapshot_InvalidatedByMutationAndByAge(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.seed("web-a", true)
	snap := NewPassSnapshot(time.Minute)
	now := time.Now()
	snap.now = func() time.Time { return now }
	ctx := context.Background()

	list := func() int {
		t.Helper()
		got, err := snap.ListByPrefix(ctx, "local", rt, "web-")
		if err != nil {
			t.Fatalf("ListByPrefix: %v", err)
		}
		return len(got)
	}
	if n := list(); n != 1 {
		t.Fatalf("first listing = %d containers, want 1", n)
	}
	rt.seed("web-b", true)
	if n := list(); n != 1 || snap.Lists() != 1 {
		t.Errorf("warm listing = %d containers after %d fetches, want the cached 1 after 1", n, snap.Lists())
	}
	snap.Invalidate("local")
	if n := list(); n != 2 || snap.Lists() != 2 {
		t.Errorf("after invalidation = %d containers after %d fetches, want 2 after 2", n, snap.Lists())
	}
	rt.seed("web-c", true)
	now = now.Add(2 * time.Minute)
	if n := list(); n != 3 || snap.Lists() != 3 {
		t.Errorf("after max age = %d containers after %d fetches, want 3 after 3", n, snap.Lists())
	}
}

func TestPassSnapshot_NodesDoNotShareListings(t *testing.T) {
	a, b := newFakeRuntime(0), newFakeRuntime(0)
	a.seed("web-1", true)
	snap := NewPassSnapshot(time.Minute)
	ctx := context.Background()

	if got, _ := snap.ListByPrefix(ctx, "a", a, "web-"); len(got) != 1 {
		t.Errorf("node a lists %d, want 1", len(got))
	}
	if got, _ := snap.ListByPrefix(ctx, "b", b, "web-"); len(got) != 0 {
		t.Errorf("node b lists %d, want 0: a listing must never leak across nodes", len(got))
	}
}

func TestSnapshotMaxAgeFromEnv(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	if got := SnapshotMaxAgeFromEnv(env("3s")); got != 3*time.Second {
		t.Errorf("3s -> %v", got)
	}
	for _, bad := range []string{"", "nope", "-1s", "0"} {
		if got := SnapshotMaxAgeFromEnv(env(bad)); got != defaultSnapshotMaxAge {
			t.Errorf("%q -> %v, want default", bad, got)
		}
	}
}

func TestPassSnapshot_PrefetchedInspectsSkipTheRuntimeForSteadyApps(t *testing.T) {
	rt := &batchRuntime{fakeRuntime: newFakeRuntime(0)}
	snap := NewPassSnapshot(time.Minute)
	var names []string
	const apps = 15
	for i := 0; i < apps; i++ {
		name := fmt.Sprintf("app%02d", i)
		target := ContainerName(name, "img:v1", "")
		rt.seed(target, true)
		names = append(names, target)
	}
	snap.Prefetch(context.Background(), "local", rt, names)

	for i := 0; i < apps; i++ {
		name := fmt.Sprintf("app%02d", i)
		c := New(name, &fakeStore{svc: runningDesired(name)}, rt, WithPassSnapshot(snap, "local"))
		result, err := c.Reconcile(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if cond := conditionOf(t, result); cond.Reason != "AlreadyRunning" {
			t.Fatalf("%s: reason = %q, want AlreadyRunning", name, cond.Reason)
		}
	}
	if rt.batches != 1 {
		t.Errorf("batch reads = %d, want 1", rt.batches)
	}
	if rt.inspectCalls != 0 {
		t.Errorf("per-app InspectByName calls = %d, want 0 for %d steady apps", rt.inspectCalls, apps)
	}
}

func TestPassSnapshot_PrefetchedCrashedContainerIsStillRestarted(t *testing.T) {
	rt := &batchRuntime{fakeRuntime: newFakeRuntime(0)}
	target := ContainerName("web", "img:v1", "")
	rt.seed(target, false)
	snap := NewPassSnapshot(time.Minute)
	snap.Prefetch(context.Background(), "local", rt, []string{target})

	c := New("web", &fakeStore{svc: runningDesired("web")}, rt, WithPassSnapshot(snap, "local"))
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Deployed" {
		t.Errorf("reason = %q, want Deployed after the restart", cond.Reason)
	}
	if cs, _ := rt.InspectByName(context.Background(), target); cs == nil || !cs.Running {
		t.Errorf("crashed container was not restarted: %+v", cs)
	}
	if rt.createCalls != 0 {
		t.Errorf("createCalls = %d, want 0: a crashed container is restarted, not recreated", rt.createCalls)
	}
}

func TestPassSnapshot_PrefetchedAbsentContainerIsCreated(t *testing.T) {
	rt := &batchRuntime{fakeRuntime: newFakeRuntime(0)}
	target := ContainerName("web", "img:v1", "")
	snap := NewPassSnapshot(time.Minute)
	snap.Prefetch(context.Background(), "local", rt, []string{target})

	c := New("web", &fakeStore{svc: runningDesired("web")}, rt, WithPassSnapshot(snap, "local"))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if rt.createCalls != 1 {
		t.Errorf("createCalls = %d, want 1", rt.createCalls)
	}
}

func TestPassSnapshot_MutationAndAgeSendInspectsBackToTheRuntime(t *testing.T) {
	rt := &batchRuntime{fakeRuntime: newFakeRuntime(0)}
	rt.seed("web-a", true)
	snap := NewPassSnapshot(time.Minute)
	now := time.Now()
	snap.now = func() time.Time { return now }
	ctx := context.Background()
	snap.Prefetch(ctx, "local", rt, []string{"web-a"})

	if cs, _ := snap.Inspect(ctx, "local", rt, "web-a"); cs == nil || rt.inspectCalls != 0 {
		t.Fatalf("fresh batch should answer: state=%v runtime calls=%d", cs, rt.inspectCalls)
	}
	if _, _ = snap.Inspect(ctx, "local", rt, "other"); rt.inspectCalls != 1 {
		t.Errorf("a name outside the batch must reach the runtime, calls=%d", rt.inspectCalls)
	}
	snap.Invalidate("local")
	if _, _ = snap.Inspect(ctx, "local", rt, "web-a"); rt.inspectCalls != 2 {
		t.Errorf("after a mutation the runtime must answer, calls=%d", rt.inspectCalls)
	}

	snap.Prefetch(ctx, "local", rt, []string{"web-a"})
	now = now.Add(2 * time.Minute)
	if _, _ = snap.Inspect(ctx, "local", rt, "web-a"); rt.inspectCalls != 3 {
		t.Errorf("past max age the runtime must answer, calls=%d", rt.inspectCalls)
	}
}

func TestPassSnapshot_PrefetchWithoutBatchSupportChangesNothing(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.seed("web-a", true)
	snap := NewPassSnapshot(time.Minute)
	snap.Prefetch(context.Background(), "local", rt, []string{"web-a"})
	if cs, _ := snap.Inspect(context.Background(), "local", rt, "web-a"); cs == nil || rt.inspectCalls != 1 {
		t.Errorf("state=%v runtime calls=%d, want the runtime to answer", cs, rt.inspectCalls)
	}
}
