package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestEffectiveHold(t *testing.T) {
	tests := []struct {
		name string
		hold time.Duration
		cfg  deploysafety.Effective
		want time.Duration
	}{
		{"configured hold wins", 3 * time.Minute, deploysafety.Effective{DrainTimeout: 15 * time.Second}, 3 * time.Minute},
		{"drain longer than hold", 0, deploysafety.Effective{DrainTimeout: 15 * time.Second}, 15 * time.Second},
		{"auto-rollback keeps the target for the watch", time.Minute, deploysafety.Effective{WatchWindow: 5 * time.Minute, AutoRollback: true}, 5 * time.Minute},
		{"watch without auto-rollback holds nothing extra", time.Minute, deploysafety.Effective{WatchWindow: 5 * time.Minute}, time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("web", &fakeStore{}, newFakeRuntime(0), WithPreviousReleaseHold(tt.hold))
			if got := c.effectiveHold(tt.cfg); got != tt.want {
				t.Errorf("effectiveHold() = %s, want %s", got, tt.want)
			}
		})
	}
}

// A rolling deploy with a drain window must not stop old replicas mid-pass:
// ingress keeps balancing over them until the pass ends.
func TestDrain_RollingKeepsOldReplicasUntilCutover(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	old := []string{ReplicaContainerName("web", "img:v1", "", 0), ReplicaContainerName("web", "img:v1", "", 1)}
	for _, name := range old {
		rt.seed(name, true)
	}
	desired := gateDesired()
	desired.Replicas, desired.Strategy = 2, strategyRolling
	for i := range 2 {
		rt.setExitState(ReplicaContainerName("web", desired.Image, "", i), &docker.ExitState{Running: true})
	}
	rec := &fakeGateRecorder{}
	opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(NewDrainTracker()))

	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, name := range old {
		if cs, _ := rt.InspectByName(context.Background(), name); cs == nil || !cs.Running {
			t.Errorf("old replica %s = %+v, want it still draining", name, cs)
		}
	}
}

// R1-1: a rollback onto the held release that is still running creates no
// container, but the newer release it replaces must still drain.
func TestDrain_RollbackOntoRunningReleaseDrainsNewer(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	for _, tt := range []struct {
		name        string
		wasServing  bool
		wantRunning bool
	}{
		{"rollback after v2 served", true, true},
		{"first pass after a restart keeps the hold rule", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(serverPort(t, srv))
			v1, v2 := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v2", "")
			rt.seed(v1, true)
			rt.seed(v2, true)
			desired := gateDesired()
			desired.Image = "img:v1"
			tracker := NewDrainTracker()
			if tt.wasServing {
				tracker.moved("web", v2)
			}
			rec := &fakeGateRecorder{}
			opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(tracker))
			c := New("web", &fakeStore{svc: desired}, rt, opts...)
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			cs, _ := rt.InspectByName(context.Background(), v2)
			if got := cs != nil && cs.Running; got != tt.wantRunning {
				t.Errorf("v2 running = %v, want %v", got, tt.wantRunning)
			}
			if tt.wantRunning && !rec.has(deploysafety.PhaseDrain, deploysafety.StatusStarted) {
				t.Error("want a drain started gate event for the replaced release")
			}
		})
	}
}

// F9: a rollback drains only the release that was serving. An older held
// release serves nothing, so it must not keep running beside the other two.
func TestDrain_RollbackStopsOlderHeldRelease(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	oldest, serving := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v1@sha256:abc", "")
	rt.seed(oldest, true)
	rt.seed(serving, true)
	now := time.Now()
	rt.containers[oldest].Created = now.Add(-3 * time.Hour)
	rt.containers[serving].Created = now.Add(-2 * time.Hour)
	desired := gateDesired()
	desired.Image = "rollback/web:dep_1"
	rt.setExitState(ContainerName("web", desired.Image, ""), &docker.ExitState{Running: true})
	rec := &fakeGateRecorder{}
	opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(NewDrainTracker()))

	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), serving); cs == nil || !cs.Running {
		t.Errorf("serving release %s = %+v, want it still draining", serving, cs)
	}
	if cs, _ := rt.InspectByName(context.Background(), oldest); cs != nil {
		t.Errorf("older release %s = %+v, want it removed", oldest, cs)
	}
}

// v3 cuts over while v1 still drains from the v2 cutover: v1 keeps its own
// window instead of being cut mid-request.
func TestDrain_OverlappingCutoverKeepsEarlierDrain(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	v1, v2 := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v2", "")
	rt.seed(v1, true)
	rt.seed(v2, true)
	now := time.Now()
	rt.containers[v1].Created = now.Add(-time.Hour)
	rt.containers[v2].Created = now.Add(-10 * time.Second)
	desired := gateDesired()
	desired.Image = "img:v3"
	rt.setExitState(ContainerName("web", desired.Image, ""), &docker.ExitState{Running: true})
	tracker := NewDrainTracker()
	tracker.moved("web", v2)
	rec := &fakeGateRecorder{}
	opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(tracker))

	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	for _, name := range []string{v1, v2} {
		if cs, _ := rt.InspectByName(context.Background(), name); cs == nil || !cs.Running {
			t.Errorf("release %s = %+v, want it still draining", name, cs)
		}
	}
}

// Replicas of the current release removed by a scale-down during an open
// drain window still owe their in-flight requests a drain.
func TestDrain_KeepsCurrentScaleDownReplicas(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	desired := gateDesired()
	base, extra := ReplicaContainerName("web", desired.Image, "", 0), ReplicaContainerName("web", desired.Image, "", 1)
	for _, name := range []string{base, extra} {
		rt.seed(name, true)
		rt.setExitState(name, &docker.ExitState{Running: true})
	}
	tracker := NewDrainTracker()
	tracker.moved("web", releaseOf(base))
	tracker.mark("web", time.Now())
	rec := &fakeGateRecorder{}
	opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(tracker))

	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), extra); cs == nil || !cs.Running {
		t.Errorf("scale-down replica %s = %+v, want it still draining", extra, cs)
	}
}

// Two previous releases created in the same second: the hold keeps the one
// that was serving, not whichever name sorts last.
func TestHeldFilter_TieKeepsServingRelease(t *testing.T) {
	a, b := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v2", "")
	serving, other := min(a, b), max(a, b)
	target := ContainerName("web", "img:v3", "")
	now := time.Now().Truncate(time.Second)
	all := []docker.ContainerState{
		{Name: serving, Running: true, Created: now.Add(-time.Second)},
		{Name: other, Running: true, Created: now.Add(-time.Second)},
		{Name: target, Running: true, Created: now},
	}
	tracker := NewDrainTracker()
	tracker.moved("web", serving)
	tracker.moved("web", target)
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithDrainTracker(tracker))

	due, _ := c.heldFilterFor(all, []string{target}, now, time.Hour)
	if len(due) != 1 || due[0].Name != other {
		t.Errorf("due = %+v, want only %s", due, other)
	}
}

// After a restart the tracker is empty: a failed newer release left stopped
// must not date the retirement of the older release that is still serving.
func TestDrain_StoppedFailedReleaseKeepsServingDrain(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	serving, failed := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v2", "")
	rt.seed(serving, true)
	rt.seed(failed, false)
	now := time.Now()
	rt.containers[serving].Created = now.Add(-3 * time.Hour)
	rt.containers[failed].Created = now.Add(-2 * time.Hour)
	desired := gateDesired()
	desired.Image = "img:v3"
	rt.setExitState(ContainerName("web", desired.Image, ""), &docker.ExitState{Running: true})
	rec := &fakeGateRecorder{}
	opts := append(gateOptions(rec, 1, 0, time.Hour), WithDrainTracker(NewDrainTracker()))

	c := New("web", &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cs, _ := rt.InspectByName(context.Background(), serving); cs == nil || !cs.Running {
		t.Errorf("serving release %s = %+v, want it still draining", serving, cs)
	}
}
