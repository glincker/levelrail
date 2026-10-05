package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_HoldsHostWhenBackendDisappears(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")
	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	tracker := ingress.NewHoldTracker(time.Minute)
	c := New(st, rt, applier, WithLogger(discardLogger()), WithHoldTracker(tracker), WithHardening(ingress.DefaultHardening()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := applier.routes(t); len(routes) != 1 {
		t.Fatalf("running: routes = %d, want 1", len(routes))
	}

	rt.mu.Lock()
	delete(rt.containers, target)
	rt.mu.Unlock()

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("backend gone: routes = %d, want 1 hold route", len(routes))
	}
	h, ok := routes[0].Handle[0].(ingress.StaticResponseHandler)
	if !ok || h.StatusCode != 503 || h.Headers["Retry-After"] == nil {
		t.Errorf("hold handler = %+v", routes[0].Handle[0])
	}
	if msg := conditionOf(t, result).Message; msg == "" {
		t.Error("condition message empty")
	}
}

func TestController_Reconcile_NeverRoutedHostIsNotHeld(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"new.example.com"}}
	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, newFakeRuntime(), applier, WithLogger(discardLogger()), WithHoldTracker(ingress.NewHoldTracker(time.Minute)))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("a never-deployed host must not trigger certificate issuance, got routes %+v", routes)
	}
}
