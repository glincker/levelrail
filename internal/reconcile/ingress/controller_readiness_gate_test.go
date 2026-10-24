package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_DialForService_NotReadyRoutesToPreviousRelease is F-002:
// a new container that is Running with a port open but hasn't passed the
// application controller's readiness probe must not receive traffic while
// an older, still-running release can serve instead.
func TestController_DialForService_NotReadyRoutesToPreviousRelease(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Domains: []string{"web.example.com"}}
	newTarget := application.ContainerName(desired.Name, desired.Image, "")
	oldTarget := application.ContainerName(desired.Name, "img:v1", "")

	rt := newFakeRuntime()
	rt.seedRunning(newTarget, 34568)
	rt.seedRunning(oldTarget, 34567)

	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition = %+v, want Reason=Routed1Services", cond)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %v, want exactly 1", routes)
	}
	handler, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("route handler is %T, want ingress.ReverseProxyHandler", routes[0].Handle[0])
	}
	if len(handler.Upstreams) != 1 || handler.Upstreams[0].Dial != "127.0.0.1:34567" {
		t.Errorf("upstream = %+v, want dial 127.0.0.1:34567 (old release), not the unready new one", handler.Upstreams)
	}
}

// TestController_DialForService_NotReadyNoPreviousRelease_NoRoute checks
// the safe failure mode when there's nothing else to fall back to: no
// route at all, never a route to the unready container.
func TestController_DialForService_NotReadyNoPreviousRelease_NoRoute(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition = %+v, want Reason=Routed0Services", cond)
	}
	if routes := applier.routes(t); len(routes) != 0 {
		t.Errorf("applied routes = %v, want none (must never route to an unready container)", routes)
	}
}

// TestController_DialForService_Ready_RoutesToTarget is the ordinary
// steady-state case, confirming the readiness gate doesn't regress it.
func TestController_DialForService_Ready_RoutesToTarget(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue || cond.Reason != "Routed1Services" {
		t.Errorf("condition = %+v, want Status=True Reason=Routed1Services", cond)
	}
	if routes := applier.routes(t); len(routes) != 1 {
		t.Errorf("applied routes = %v, want exactly 1", routes)
	}
}

// TestController_DialForService_NotReadyKeepsNewestServingRelease covers a
// failed third deploy: v1 is still held, v2 was serving, v3 never became
// ready. Traffic must stay on v2, not drop back to the older v1.
func TestController_DialForService_NotReadyKeepsNewestServingRelease(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v3", Port: 80, Domains: []string{"web.example.com"}}
	v3 := application.ContainerName(desired.Name, "img:v3", "")
	v2 := application.ContainerName(desired.Name, "img:v2", "")
	v1 := application.ContainerName(desired.Name, "img:v1", "")

	rt := newFakeRuntime()
	rt.seedRunning(v1, 34561)
	rt.seedRunning(v2, 34562)
	rt.seedRunning(v3, 34563)
	// A per-deploy preview service named "web-dep209p" shares the "web-"
	// prefix and is the newest container; it must never be a fallback.
	sibling := application.ContainerName("web-dep209p", "levelrail-rollback/web:dep_209", "")
	rt.seedRunning(sibling, 34564)
	base := time.Now().Add(-10 * time.Minute)
	rt.containers[sibling].Created = base.Add(4 * time.Minute)
	rt.containers[v1].Created = base
	rt.containers[v2].Created = base.Add(3 * time.Minute)
	rt.containers[v3].Created = base.Add(6 * time.Minute)

	st := &fakeStore{
		services:         []store.DesiredService{desired},
		notReadyServices: map[string]bool{"web": true},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %v, want exactly 1", routes)
	}
	handler, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("route handler is %T, want ingress.ReverseProxyHandler", routes[0].Handle[0])
	}
	if len(handler.Upstreams) != 1 || handler.Upstreams[0].Dial != "127.0.0.1:34562" {
		t.Errorf("upstream = %+v, want dial 127.0.0.1:34562 (v2, last serving), not v1", handler.Upstreams)
	}
}
