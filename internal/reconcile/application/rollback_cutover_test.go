package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
)

func (f *fakeGateRecorder) count(phase, status string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.events {
		if e.Phase == phase && e.Status == status {
			n++
		}
	}
	return n
}

// The post-deploy watch arms from the cutover gate event, so an instant
// rollback onto a held running release must record one too.
func TestCutoverRecordedForRollbackToHeldRelease(t *testing.T) {
	tests := []struct {
		name         string
		secondImage  string
		wantCutovers int
	}{
		{"rollback to the held previous release", "img:v1", 2},
		{"same release reconciled again", "img:v2", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := alwaysHealthy()
			defer srv.Close()
			rt := newFakeRuntime(serverPort(t, srv))
			desired := gateDesired()
			desired.Strategy = strategyBlueGreen
			rt.setExitState(ContainerName("web", desired.Image, ""), &docker.ExitState{Running: true})
			rec := &fakeGateRecorder{}
			opts := append(gateOptions(rec, 1, 0, 0), WithPreviousReleaseHold(time.Hour))
			c := New("web", &fakeStore{svc: desired}, rt, opts...)

			ctx := context.Background()
			if _, err := c.Reconcile(ctx); err != nil {
				t.Fatalf("first Reconcile() error = %v", err)
			}
			// The previous release is still up as the held rollback target;
			// the fake stamps no creation time, which the hold keys on.
			rt.seed(ContainerName("web", "img:v1", ""), true)
			rt.mu.Lock()
			rt.containers[ContainerName("web", "img:v1", "")].Created = time.Now().Add(-time.Minute)
			rt.containers[ContainerName("web", "img:v2", "")].Created = time.Now()
			rt.mu.Unlock()
			desired.Image = tt.secondImage
			if _, err := c.Reconcile(ctx); err != nil {
				t.Fatalf("second Reconcile() error = %v", err)
			}
			if got := rec.count(deploysafety.PhaseCutover, deploysafety.StatusPassed); got != tt.wantCutovers {
				t.Errorf("cutover events = %d, want %d: %+v", got, tt.wantCutovers, rec.events)
			}
		})
	}
}
