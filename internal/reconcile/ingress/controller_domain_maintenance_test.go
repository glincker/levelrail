package ingress

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_DomainMaintenance_NoRunningContainer_StillRouted
// is this feature's own defining behavior, the one thing that
// distinguishes it from every other per-domain toggle in this
// controller: a domain in maintenance mode is routed even when its
// service has no running container at all, unlike an ordinary
// ProxyRoute, which dialForService would otherwise skip entirely,
// leaving the domain unrouted in Caddy rather than showing an operator-
// controlled response.
func TestController_Reconcile_DomainMaintenance_NoRunningContainer_StillRouted(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"down.example.com"}}
	// No seeded container at all: dialForService would return ok=false.
	rt := newFakeRuntime()

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		maintenance: []string{"down.example.com"},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition reason = %q, want Routed1Services: a maintenance-mode domain must be routed even with zero running containers", cond.Reason)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %+v, want exactly 1", routes)
	}
	route := routes[0]
	if len(route.Handle) != 1 {
		t.Fatalf("route.Handle = %+v, want exactly 1 handler (static_response only, no reverse_proxy)", route.Handle)
	}
	handler, ok := route.Handle[0].(ingress.StaticResponseHandler)
	if !ok {
		t.Fatalf("route.Handle[0] is %T, want ingress.StaticResponseHandler", route.Handle[0])
	}
	if handler.StatusCode != 503 {
		t.Errorf("handler.StatusCode = %d, want 503", handler.StatusCode)
	}
}

// TestController_Reconcile_DomainMaintenance_MixedDomains_SplitsIntoMaintenanceAndProxyRoutes
// proves one service with one domain in maintenance and one still-active
// domain produces two separate routes: the maintenance domain alone
// with a static_response handler, and the active domain routed to its
// real backend exactly as before this feature existed.
func TestController_Reconcile_DomainMaintenance_MixedDomains_SplitsIntoMaintenanceAndProxyRoutes(t *testing.T) {
	desired := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"down.example.com", "up.example.com"},
	}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		maintenance: []string{"down.example.com"},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("applied routes = %+v, want exactly 2 (one maintenance, one active)", routes)
	}

	var sawMaintenance, sawActive bool
	for _, route := range routes {
		hosts := route.Match[0].Host
		switch {
		case len(hosts) == 1 && hosts[0] == "down.example.com":
			sawMaintenance = true
			if _, ok := route.Handle[0].(ingress.StaticResponseHandler); !ok {
				t.Errorf("down.example.com route.Handle[0] is %T, want ingress.StaticResponseHandler", route.Handle[0])
			}
		case len(hosts) == 1 && hosts[0] == "up.example.com":
			sawActive = true
			if _, ok := route.Handle[0].(ingress.ReverseProxyHandler); !ok {
				t.Errorf("up.example.com route.Handle[0] is %T, want ingress.ReverseProxyHandler", route.Handle[0])
			}
		default:
			t.Errorf("unexpected route hosts %+v", hosts)
		}
	}
	if !sawMaintenance || !sawActive {
		t.Errorf("routes = %+v, want one down.example.com maintenance route and one up.example.com proxy route", routes)
	}
}

// TestController_Reconcile_DomainMaintenance_ListError_Fails proves a
// store failure listing maintenance domains fails the whole reconcile
// pass (unlike a per-domain resolve failure elsewhere in this
// controller): there is no safe partial state to fall back to when this
// controller cannot even tell which domains are supposed to be down.
func TestController_Reconcile_DomainMaintenance_ListError_Fails(t *testing.T) {
	st := &fakeStore{maintenanceErr: errors.New("db unreachable")}
	applier := &fakeApplier{}
	c := New(st, newFakeRuntime(), applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want an error")
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
}
