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
