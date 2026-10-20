package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_ApplyFailsThenRecovers checks a rejected Caddy
// config leaves no cached "already applied" state: the next pass rebuilds
// and applies the same routes.
func TestController_Reconcile_ApplyFailsThenRecovers(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	applier := &fakeApplier{applyErr: errors.New("caddy rejected config")}
	c := New(&fakeStore{services: []store.DesiredService{desired}}, rt, applier, WithLogger(discardLogger()))

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("first pass error = nil, want the apply failure")
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionFalse || cond.Reason != "ApplyFailed" {
		t.Fatalf("first pass condition = %+v, want False/ApplyFailed", cond)
	}

	applier.applyErr = nil
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("retry condition = %+v, want True", cond)
	}
	if applier.calls != 2 {
		t.Errorf("Apply calls = %d, want 2 (retry must re-apply, not skip)", applier.calls)
	}
	if got := len(applier.routes(t)); got != 1 {
		t.Errorf("routes applied on retry = %d, want 1", got)
	}
}
