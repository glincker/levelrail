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

// TestController_Reconcile_DomainErrorPages_AddsHandleResponseAndSubroute
// proves a domain with configured error pages gets a route whose
// reverse_proxy handler carries handle_response for the configured codes,
// wrapped in a subroute carrying the same codes as error routes: the
// same JSON shape internal/ingress.BuildRoutesConfig's own tests assert
// on, now proven end-to-end through the reconciler.
func TestController_Reconcile_DomainErrorPages_AddsHandleResponseAndSubroute(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		errorPages: []store.DomainErrorPage{
			{Domain: "app.example.com", StatusCode: 404, Body: "<h1>not found</h1>"},
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %+v, want exactly 1", routes)
	}
	route := routes[0]
	if len(route.Handle) != 1 {
		t.Fatalf("route.Handle = %+v, want exactly 1 handler (subroute)", route.Handle)
	}
	sub, ok := route.Handle[0].(ingress.SubrouteHandler)
	if !ok {
		t.Fatalf("route.Handle[0] is %T, want ingress.SubrouteHandler", route.Handle[0])
	}
	if sub.Errors == nil || len(sub.Errors.Routes) != 1 {
		t.Fatalf("sub.Errors = %+v, want exactly one error route for the 404 mapping", sub.Errors)
	}
	if len(sub.Routes) != 1 || len(sub.Routes[0].Handle) != 1 {
		t.Fatalf("sub.Routes = %+v, want exactly one wrapped route with one handler", sub.Routes)
	}
	rp, ok := sub.Routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("sub.Routes[0].Handle[0] is %T, want ingress.ReverseProxyHandler", sub.Routes[0].Handle[0])
	}
	if len(rp.HandleResponse) != 1 {
		t.Fatalf("rp.HandleResponse = %+v, want exactly one entry for the 404 mapping", rp.HandleResponse)
	}
	if got := rp.HandleResponse[0].Match.StatusCode; len(got) != 1 || got[0] != 404 {
		t.Errorf("HandleResponse[0].Match.StatusCode = %v, want [404]", got)
	}
}

// TestController_Reconcile_DomainErrorPages_NoConfig_PlainReverseProxy
// proves a domain with no error pages configured reproduces this
// controller's behavior before this feature existed exactly: a single
// reverse_proxy handler, no subroute wrapping at all.
func TestController_Reconcile_DomainErrorPages_NoConfig_PlainReverseProxy(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"app.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{services: []store.DesiredService{desired}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %+v, want exactly 1", routes)
	}
	if _, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler); !ok {
		t.Errorf("route.Handle[0] is %T, want a plain ingress.ReverseProxyHandler, no subroute wrapping", routes[0].Handle[0])
	}
}

// TestController_Reconcile_DomainErrorPages_ListError_Fails mirrors
// TestController_Reconcile_DomainRedirect_ListError_Fails: a store
// failure listing error pages fails the whole reconcile pass.
func TestController_Reconcile_DomainErrorPages_ListError_Fails(t *testing.T) {
	st := &fakeStore{errorPagesErr: errors.New("db unreachable")}
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
