package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/deploysafety"
	"github.com/glincker/levelrail/internal/docker"
)

// A superseded release whose removal fails once must still be removed on the
// next pass, not survive until the app is deleted.
func TestRetire_SupersededReleaseRemovalRetriedAfterFailure(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	cfg := deploysafety.Effective{SuccessThreshold: 1, DrainTimeout: 15 * time.Second}
	rt := newFakeRuntime(serverPort(t, srv))
	p := clockedPass{t: t, rt: rt, tracker: NewDrainTracker(), cfg: cfg, hold: 3 * time.Minute}
	desired := *gateDesired()
	v1, v2 := ContainerName("web", "img:v1", ""), ContainerName("web", "img:v2", "")
	for _, name := range []string{v1, v2} {
		rt.setExitState(name, &docker.ExitState{Running: true})
	}
	t0 := time.Now().Truncate(time.Second)
	desired.Image = "img:v1"
	p.run(t0.Add(-time.Hour), desired)
	desired.Image = "img:v2"
	p.run(t0, desired)

	rt.mu.Lock()
	rt.removeErr = errors.New("remove failed")
	rt.mu.Unlock()
	failAt := t0.Add(4 * time.Minute)
	c := New("web", &fakeStore{svc: &desired}, rt,
		WithStabilityGate(cfg, nil), WithGateRecorder(&fakeGateRecorder{}), WithGateTick(10*time.Millisecond),
		WithReadyBudget(2*time.Second), WithDrainTracker(p.tracker), WithPreviousReleaseHold(p.hold))
	c.now = func() time.Time { return failAt }
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want the removal failure")
	}
	if cs, _ := rt.InspectByName(context.Background(), v1); cs == nil {
		t.Fatal("v1 gone despite the failed removal")
	}

	rt.mu.Lock()
	rt.removeErr = nil
	rt.mu.Unlock()
	p.run(failAt.Add(time.Minute), desired)
	if cs, _ := rt.InspectByName(context.Background(), v1); cs != nil {
		t.Fatalf("v1 = %+v after the retry, want it removed", cs)
	}
	if !p.running(v2) {
		t.Fatal("current release v2 not running")
	}
}
