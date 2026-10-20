package ingress

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_RegistryRoute_Added is
// TestController_Reconcile_PlatformDomainRoute_Added's exact counterpart
// for the built-in registry: an enabled registry with a Host set adds a
// reverse-proxy route to WithRegistryDial's configured target.
func TestController_Reconcile_RegistryRoute_Added(t *testing.T) {
	st := &fakeStore{registry: store.RegistrySettings{Enabled: true, Host: "registry.example.com"}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithRegistryDial("127.0.0.1:5540"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services (the registry route counts)", cond.Reason)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want exactly one", routes)
	}
	if len(routes[0].Match) != 1 || len(routes[0].Match[0].Host) != 1 || routes[0].Match[0].Host[0] != "registry.example.com" {
		t.Errorf("route match = %+v, want Host=[registry.example.com]", routes[0].Match)
	}
	handler, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("Handle[0] = %T, want ingress.ReverseProxyHandler (no basic_auth: the registry enforces its own htpasswd auth)", routes[0].Handle[0])
	}
	if handler.Upstreams[0].Dial != "127.0.0.1:5540" {
		t.Errorf("route dial = %q, want 127.0.0.1:5540", handler.Upstreams[0].Dial)
	}
}

// TestController_Reconcile_RegistryRoute_NoDial_Skipped is
// TestController_Reconcile_PlatformDomainRoute_NoDashboardDial_Skipped's
// exact counterpart: an enabled registry with a Host set but no
// WithRegistryDial wiring fails closed, no route and no error.
func TestController_Reconcile_RegistryRoute_NoDial_Skipped(t *testing.T) {
	st := &fakeStore{registry: store.RegistrySettings{Enabled: true, Host: "registry.example.com"}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger())) // no WithRegistryDial

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition.Reason = %q, want Routed0Services", cond.Reason)
	}
}

// TestController_Reconcile_RegistryRoute_Disabled_Skipped proves a
// disabled registry (the default, matching a fresh migration's seeded
// row) never gets a route even with WithRegistryDial configured.
func TestController_Reconcile_RegistryRoute_Disabled_Skipped(t *testing.T) {
	st := &fakeStore{registry: store.RegistrySettings{Enabled: false, Host: "registry.example.com"}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithRegistryDial("127.0.0.1:5540"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition.Reason = %q, want Routed0Services", cond.Reason)
	}
}

// TestController_Reconcile_RegistryRoute_ConflictsWithService_Skipped is
// TestController_Reconcile_PlatformDomainRoute_ConflictsWithService_Skipped's
// exact counterpart: a registry Host colliding with a real app's domain
// loses, the same "skip the loser, never build two routes for one host"
// rule the rest of this controller enforces.
func TestController_Reconcile_RegistryRoute_ConflictsWithService_Skipped(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"registry.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		registry: store.RegistrySettings{Enabled: true, Host: "registry.example.com"},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithRegistryDial("127.0.0.1:5540"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services (only the app's route, not a second one for the registry)", cond.Reason)
	}
	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want exactly one (the app's, not the registry's)", routes)
	}
	handler, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("Handle[0] = %T, want ingress.ReverseProxyHandler", routes[0].Handle[0])
	}
	if handler.Upstreams[0].Dial == "127.0.0.1:5540" {
		t.Errorf("route dials the registry target, want the app's container dial: the app's route must win the conflict")
	}
}
