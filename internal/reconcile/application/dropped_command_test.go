package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// An app update that loses its command leaves an HTTP service whose image
// default exits 0 at once; with no probe and no run-to-completion restart it must never read Ready.
func TestExitedReadiness_DroppedCommandExitZeroIsNotReady(t *testing.T) {
	rt := &exitOnStartRuntime{fakeRuntime: newFakeRuntime(0), exitCode: 0, exitImg: "img:v1"}
	svc := &store.DesiredService{Name: "web", Image: "img:v1", Port: 8080, Replicas: 1}
	c := New("web", &fakeStore{svc: svc}, rt, WithStartupWatch(0))

	for pass := 0; pass < 3; pass++ {
		res, err := c.Reconcile(context.Background())
		if cond := conditionOf(t, res); err == nil || cond.Status == reconcile.ConditionTrue {
			t.Fatalf("pass %d: err = %v condition = %+v, want not Ready", pass, err, cond)
		}
	}
}
