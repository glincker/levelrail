package ingress

// This file: build.type: static (static sites served by the embedded
// ingress directly, no container) coverage for Controller,
// split out of controller_test.go purely to keep that file's line count
// down; it shares every fake (fakeStore, fakeRuntime, fakeApplier) and
// helper (conditionOf, discardLogger) controller_test.go already
// defines, same package, same test binary.

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_StaticSite_RouteAppears is the core static-site
// case: no container, no Docker inspection at all, a file_server route
// built straight from store.StaticSite.RootDir. This is the design's
// "served by the embedded Caddy directly with no container" bypass of
// the reconciler, exercised end to end through this controller.
func TestController_Reconcile_StaticSite_RouteAppears(t *testing.T) {
	st := &fakeStore{staticSites: []store.StaticSite{
		{Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/static/docs/abc1234"},
	}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed1Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed1Services", cond)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %v, want exactly 1", routes)
	}
	if len(routes[0].Match) != 1 || len(routes[0].Match[0].Host) != 1 || routes[0].Match[0].Host[0] != "docs.example.com" {
		t.Errorf("route match = %+v, want host docs.example.com", routes[0].Match)
	}
	handler, ok := routes[0].Handle[0].(ingress.FileServerHandler)
	if !ok {
		t.Fatalf("route handler is %T, want ingress.FileServerHandler", routes[0].Handle[0])
	}
	if handler.Root != "/data/static/docs/abc1234" {
		t.Errorf("handler.Root = %q, want /data/static/docs/abc1234", handler.Root)
	}
}

// TestController_Reconcile_StaticSiteNoDomains_Skipped mirrors a
// container service declaring no domains: nothing to route, no error.
// internal/spec's Validate() already requires a static service to have
// no port, but says nothing about domains being non-empty, so this
// controller must handle it the same defensive way it already handles a
// domain-less container service.
func TestController_Reconcile_StaticSiteNoDomains_Skipped(t *testing.T) {
	st := &fakeStore{staticSites: []store.StaticSite{
		{Name: "docs", RootDir: "/data/static/docs"},
	}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition.Reason = %q, want Routed0Services", cond.Reason)
	}
}

// TestController_Reconcile_ContainerAndStaticSite_BothRouted proves the
// two resource kinds coexist on the shared server: one reverse_proxy
// route from a running container, one file_server route from a static
// site with no container at all, applied in the same Caddy config.
func TestController_Reconcile_ContainerAndStaticSite_BothRouted(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image)

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		staticSites: []store.StaticSite{{Name: "docs", Domains: []string{"docs.example.com"}, RootDir: "/data/static/docs"}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed2Services" {
		t.Errorf("condition.Reason = %q, want Routed2Services", cond.Reason)
	}

	routes := applier.routes(t)
	if len(routes) != 2 {
		t.Fatalf("applied routes = %v, want exactly 2", routes)
	}
	var sawProxy, sawFile bool
	for _, r := range routes {
		switch r.Handle[0].(type) {
		case ingress.ReverseProxyHandler:
			sawProxy = true
		case ingress.FileServerHandler:
			sawFile = true
		}
	}
	if !sawProxy || !sawFile {
		t.Errorf("routes = %+v, want one ReverseProxyHandler and one FileServerHandler", routes)
	}
}

// TestController_Reconcile_StaticSiteAndContainer_DomainCollision_LoserSkipped
// is the cross-table case migrations/0015's own comment exists for: a
// static site and a container service both claiming the same host. This
// is defense-in-depth (store.SaveStaticSite/store.SaveDesiredService's
// own domainOwner check should prevent it from ever being written), but
// this controller must still never build two routes for the same host if
// it happens.
func TestController_Reconcile_StaticSiteAndContainer_DomainCollision_LoserSkipped(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"shared.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image)

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		staticSites: []store.StaticSite{{Name: "docs", Domains: []string{"shared.example.com"}, RootDir: "/data/static/docs"}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: a domain collision is logged and skipped, not a reconcile failure", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services: only the first-processed resource keeps the host", cond.Reason)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %v, want exactly 1 (the loser must be skipped, not both applied)", routes)
	}
	// Services are processed before static sites (Reconcile's own order),
	// so the container service wins this collision and the static site is
	// the one skipped.
	if _, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler); !ok {
		t.Errorf("surviving route handler is %T, want ingress.ReverseProxyHandler (the container service, processed first)", routes[0].Handle[0])
	}
}

// TestController_Reconcile_StaticSitesListError_Fails proves a genuine
// failure to list static sites fails the whole reconcile, unlike a
// missing container backend, which is a normal transient state, not an
// error.
func TestController_Reconcile_StaticSitesListError_Fails(t *testing.T) {
	st := &fakeStore{staticSitesErr: errors.New("database is locked")}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the static sites list failure to propagate")
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
	if applier.calls != 0 {
		t.Errorf("Apply calls = %d, want 0: a failed list must never reach Caddy", applier.calls)
	}
}
