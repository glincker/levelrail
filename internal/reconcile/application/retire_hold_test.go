package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// clockedPass runs one reconcile pass at a fake time with a fresh
// controller, as production builds one per pass around a shared tracker.
type clockedPass struct {
	t       *testing.T
	rt      *fakeRuntime
	tracker *DrainTracker
	cfg     deploysafety.Effective
	hold    time.Duration
}

func (p clockedPass) run(at time.Time, desired store.DesiredService) {
	p.t.Helper()
	opts := []Option{
		WithStabilityGate(p.cfg, nil), WithGateRecorder(&fakeGateRecorder{}), WithGateTick(10 * time.Millisecond),
		WithReadyBudget(2 * time.Second), WithDrainTracker(p.tracker), WithPreviousReleaseHold(p.hold),
	}
	c := New("web", &fakeStore{svc: &desired}, p.rt, opts...)
	c.now = func() time.Time { return at }
	if _, err := c.Reconcile(context.Background()); err != nil {
		p.t.Fatalf("Reconcile() at %s error = %v", at.Format(time.TimeOnly), err)
	}
	p.rt.mu.Lock()
	for _, cs := range p.rt.containers {
		if cs.Created.IsZero() {
			cs.Created = at
		}
	}
	p.rt.mu.Unlock()
}

func (p clockedPass) running(name string) bool {
	cs, _ := p.rt.InspectByName(context.Background(), name)
	return cs != nil && cs.Running
}

// A release superseded by a deploy, then by a rollback, is removed once the
// effective hold has elapsed, and not before.
func TestRetire_SupersededReleaseRemovedAfterHold(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	base := deploysafety.Effective{SuccessThreshold: 1, DrainTimeout: 15 * time.Second}
	watched := base
	watched.WatchWindow, watched.AutoRollback = 5*time.Minute, true
	for _, tt := range []struct {
		name     string
		cfg      deploysafety.Effective
		hold     time.Duration
		wantHeld time.Duration
	}{
		{"configured hold", base, 3 * time.Minute, 3 * time.Minute},
		{"auto-rollback watch outlasts the hold", watched, 3 * time.Minute, 5 * time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(serverPort(t, srv))
			p := clockedPass{t: t, rt: rt, tracker: NewDrainTracker(), cfg: tt.cfg, hold: tt.hold}
			desired := *gateDesired()
			v1, v2 := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v2", "")
			rb := ContainerName("web", "levelrail-rollback/web:dep_1", "")
			for _, name := range []string{v1, v2, rb} {
				rt.setExitState(name, &docker.ExitState{Running: true})
			}
			t0 := time.Now().Truncate(time.Second)

			desired.Image = "img:v1"
			p.run(t0.Add(-time.Hour), desired)
			desired.Image = "img:v2"
			p.run(t0, desired)
			p.run(t0.Add(tt.wantHeld-time.Second), desired)
			if !p.running(v1) {
				t.Fatalf("v1 removed before its hold of %s ended", tt.wantHeld)
			}
			p.run(t0.Add(tt.wantHeld+time.Second), desired)
			if cs, _ := rt.InspectByName(context.Background(), v1); cs != nil {
				t.Fatalf("v1 = %+v after the hold, want it removed", cs)
			}

			r0 := t0.Add(time.Hour)
			desired.Image = "levelrail-rollback/web:dep_1"
			p.run(r0, desired)
			p.run(r0.Add(tt.wantHeld-time.Second), desired)
			if !p.running(v2) {
				t.Fatalf("v2 removed before its hold of %s after the rollback", tt.wantHeld)
			}
			p.run(r0.Add(tt.wantHeld+time.Second), desired)
			if cs, _ := rt.InspectByName(context.Background(), v2); cs != nil {
				t.Fatalf("v2 = %+v after the rollback hold, want it removed", cs)
			}
			if !p.running(rb) {
				t.Fatal("rollback release not running")
			}
		})
	}
}
