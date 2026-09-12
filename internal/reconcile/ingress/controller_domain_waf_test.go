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

// TestController_Reconcile_DomainWAF_Enabled_GetsWAFHandler proves a
// domain with WAF enabled gets a route carrying the waf handler ahead of
// reverse_proxy, while a sibling domain on the same service with no
// domain_waf row is unaffected: opting one domain in must never change
// another domain's routing.
func TestController_Reconcile_DomainWAF_Enabled_GetsWAFHandler(t *testing.T) {
	desired := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"protected.example.com", "open.example.com"},
	}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		waf: []store.DomainWAF{
			{Domain: "protected.example.com", WAFEnabled: true, WAFMode: store.DomainWAFModeBlock},
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed2Services" {
		t.Errorf("condition reason = %q, want Routed2Services (one route per host, since WAF splits protected.example.com into its own route)", cond.Reason)
	}

	routes := applier.routes(t)
	var protectedRoute, openRoute *ingress.Route
	for i := range routes {
		host := routes[i].Match[0].Host[0]
		switch host {
		case "protected.example.com":
			protectedRoute = &routes[i]
		case "open.example.com":
			openRoute = &routes[i]
		}
	}
	if protectedRoute == nil || openRoute == nil {
		t.Fatalf("routes = %+v, want one route per host", routes)
	}

	if len(protectedRoute.Handle) != 2 {
		t.Fatalf("protected route handle = %+v, want [waf, reverse_proxy]", protectedRoute.Handle)
	}
	waf, ok := protectedRoute.Handle[0].(ingress.CorazaWAFHandler)
	if !ok {
		t.Fatalf("protected route handle[0] is %T, want ingress.CorazaWAFHandler", protectedRoute.Handle[0])
	}
	if !waf.LoadOWASPCRS {
		t.Errorf("LoadOWASPCRS = false, want true")
	}

	if len(openRoute.Handle) != 1 {
		t.Errorf("open route handle = %+v, want exactly [reverse_proxy], no waf handler for an unconfigured domain", openRoute.Handle)
	}
	if _, ok := openRoute.Handle[0].(ingress.ReverseProxyHandler); !ok {
		t.Errorf("open route handle[0] is %T, want ingress.ReverseProxyHandler", openRoute.Handle[0])
	}
}

// TestController_Reconcile_DomainWAF_RateLimitOnly_NoWAFHandler proves
// rate limiting can be enabled independently of the WAF: a domain with
// RateLimitRPS set but WAFEnabled false gets a rate_limit handler and no
// waf handler at all.
func TestController_Reconcile_DomainWAF_RateLimitOnly_NoWAFHandler(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"limited.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		waf:      []store.DomainWAF{{Domain: "limited.example.com", RateLimitRPS: 20, RateLimitBurst: 40}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want exactly 1", routes)
	}
	if len(routes[0].Handle) != 2 {
		t.Fatalf("handle = %+v, want [rate_limit, reverse_proxy]", routes[0].Handle)
	}
	if _, ok := routes[0].Handle[0].(ingress.RateLimitHandler); !ok {
		t.Fatalf("handle[0] is %T, want ingress.RateLimitHandler", routes[0].Handle[0])
	}
}

// TestController_Reconcile_DomainWAF_DisabledRow_NoHandlerAtAll proves a
// domain_waf row with both WAFEnabled false and RateLimitRPS 0 (an
// operator who enabled and then fully disabled both features, so the
// row still exists) behaves exactly like no row at all: no waf or
// rate_limit handler, joining the shared open route.
func TestController_Reconcile_DomainWAF_DisabledRow_NoHandlerAtAll(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		waf:      []store.DomainWAF{{Domain: "web.example.com", WAFEnabled: false, WAFMode: store.DomainWAFModeDetect, RateLimitRPS: 0}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 1 || len(routes[0].Handle) != 1 {
		t.Fatalf("routes = %+v, want exactly 1 route with exactly [reverse_proxy]", routes)
	}
}

// TestController_Reconcile_DomainWAF_And_BasicAuth_Together proves the
// two independent per-domain toggles compose on the same host: the
// resulting route carries both handlers, waf ahead of authentication
// ahead of reverse_proxy, matching internal/ingress.BuildRoutesConfig's
// own documented ordering.
func TestController_Reconcile_DomainWAF_And_BasicAuth_Together(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"both.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	resolver := &fakePasswordResolver{passwords: map[string]string{
		store.DomainBasicAuthSecretsKey("both.example.com"): "s3cret", //nolint:gosec // fake fixture, not a real credential
	}}

	st := &fakeStore{
		services:  []store.DesiredService{desired},
		basicAuth: []store.DomainBasicAuth{{Domain: "both.example.com", Username: "operator"}},
		waf:       []store.DomainWAF{{Domain: "both.example.com", WAFEnabled: true, WAFMode: store.DomainWAFModeBlock}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainBasicAuthSecrets(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want exactly 1", routes)
	}
	handle := routes[0].Handle
	if len(handle) != 3 {
		t.Fatalf("handle = %+v, want [waf, authentication, reverse_proxy]", handle)
	}
	if _, ok := handle[0].(ingress.CorazaWAFHandler); !ok {
		t.Errorf("handle[0] is %T, want ingress.CorazaWAFHandler", handle[0])
	}
	if _, ok := handle[1].(ingress.BasicAuthHandler); !ok {
		t.Errorf("handle[1] is %T, want ingress.BasicAuthHandler", handle[1])
	}
	if _, ok := handle[2].(ingress.ReverseProxyHandler); !ok {
		t.Errorf("handle[2] is %T, want ingress.ReverseProxyHandler", handle[2])
	}
}

// TestController_Reconcile_ListDomainWAFError is the partial-failure
// case every reconciler test must cover (a test for the case where the
// operation half-succeeded): every other store call succeeds, only the
// new ListDomainWAF call fails. The whole reconcile must still fail (no
// route built with only part of the desired-state picture is safer than
// routing without knowing which domains opted into a security control),
// and Apply must never be called, mirroring
// TestController_Reconcile_GetIngressSettingsError's identical shape for
// a different store call.
func TestController_Reconcile_ListDomainWAFError(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		wafErr:   errors.New("domain_waf table locked"),
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the domain waf list failure to surface as a real error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
	if applier.calls != 0 {
		t.Errorf("Apply calls = %d, want 0: a half-known desired state must never be applied", applier.calls)
	}
}
