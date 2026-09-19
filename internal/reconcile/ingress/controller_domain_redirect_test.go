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

// TestController_Reconcile_DomainRedirect_NoRunningContainer_StillRouted
// mirrors TestController_Reconcile_DomainMaintenance_NoRunningContainer_StillRouted's
// own defining behavior for a different per-domain toggle: a redirected
// domain is routed even when its service has no running container at
// all, since a static_response handler needs no backend to dial.
func TestController_Reconcile_DomainRedirect_NoRunningContainer_StillRouted(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"}}
	rt := newFakeRuntime()

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		redirects: []store.DomainRedirect{{Domain: "www.example.com", TargetURL: "https://example.com", StatusCode: store.DomainRedirectPermanent}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition reason = %q, want Routed1Services: a redirected domain must be routed even with zero running containers", cond.Reason)
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
	if handler.StatusCode != store.DomainRedirectPermanent {
		t.Errorf("handler.StatusCode = %d, want %d", handler.StatusCode, store.DomainRedirectPermanent)
	}
	if got := handler.Headers["Location"]; len(got) != 1 || got[0] != "https://example.com" {
		t.Errorf("Location header = %v, want [\"https://example.com\"]", handler.Headers["Location"])
	}
}

// TestController_Reconcile_DomainRedirect_MixedDomains_SplitsIntoRedirectAndProxyRoutes
// mirrors the maintenance controller test's identical shape: one service
// with one redirected domain and one still-active domain produces two
// separate routes.
func TestController_Reconcile_DomainRedirect_MixedDomains_SplitsIntoRedirectAndProxyRoutes(t *testing.T) {
	desired := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"www.example.com", "up.example.com"},
	}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		redirects: []store.DomainRedirect{{Domain: "www.example.com", TargetURL: "https://example.com", StatusCode: store.DomainRedirectPermanent}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("applied routes = %+v, want exactly 2 (one redirect, one active)", routes)
	}

	var sawRedirect, sawActive bool
	for _, route := range routes {
		hosts := route.Match[0].Host
		switch {
		case len(hosts) == 1 && hosts[0] == "www.example.com":
			sawRedirect = true
			if _, ok := route.Handle[0].(ingress.StaticResponseHandler); !ok {
				t.Errorf("www.example.com route.Handle[0] is %T, want ingress.StaticResponseHandler", route.Handle[0])
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
	if !sawRedirect || !sawActive {
		t.Errorf("routes = %+v, want one www.example.com redirect route and one up.example.com proxy route", routes)
	}
}

// TestController_Reconcile_MaintenanceTakesPrecedenceOverRedirect proves
// this feature's documented precedence decision: a domain with both
// maintenance mode and a redirect configured stays in maintenance,
// producing exactly one route with the maintenance response, never a
// redirect and never two conflicting routes for the same host.
func TestController_Reconcile_MaintenanceTakesPrecedenceOverRedirect(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"www.example.com"}}
	rt := newFakeRuntime()

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		maintenance: []string{"www.example.com"},
		redirects:   []store.DomainRedirect{{Domain: "www.example.com", TargetURL: "https://example.com", StatusCode: store.DomainRedirectPermanent}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %+v, want exactly 1: maintenance and redirect must never both route the same host", routes)
	}
	handler, ok := routes[0].Handle[0].(ingress.StaticResponseHandler)
	if !ok {
		t.Fatalf("route.Handle[0] is %T, want ingress.StaticResponseHandler", routes[0].Handle[0])
	}
	if handler.StatusCode != 503 {
		t.Errorf("handler.StatusCode = %d, want 503: maintenance mode must win over a configured redirect", handler.StatusCode)
	}
	if _, hasLocation := handler.Headers["Location"]; hasLocation {
		t.Errorf("handler.Headers = %+v, want no Location header: this must be the maintenance response, not the redirect", handler.Headers)
	}
}

// TestController_Reconcile_DomainRedirect_ListError_Fails mirrors
// TestController_Reconcile_DomainMaintenance_ListError_Fails: a store
// failure listing redirects fails the whole reconcile pass.
func TestController_Reconcile_DomainRedirect_ListError_Fails(t *testing.T) {
	st := &fakeStore{redirectsErr: errors.New("db unreachable")}
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
