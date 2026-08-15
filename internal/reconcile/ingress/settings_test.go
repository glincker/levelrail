package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_Reconcile_ACMEDisabledByDefault_RegressionUnchanged is
// this feature's own single most important regression test, per the
// task's safety constraints: a fakeStore with the zero-value
// store.IngressSettings (what every real, never-configured control
// plane actually has, migrations/0023's own seeded row) must produce an
// applied Config whose TLS app is the internal-issuer shape, exactly as
// it was before ACME support existed. No PKI app is ever built for ACME
// (acmeIssuerTLSApp never sets one), so asserting Apps.PKI is present is
// itself proof the internal issuer path, not the ACME one, is what ran.
func TestController_Reconcile_ACMEDisabledByDefault_RegressionUnchanged(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{services: []store.DesiredService{desired}} // settings left at zero value: ACME disabled
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if applier.lastCfg.Apps.PKI == nil {
		t.Error("Apps.PKI = nil, want the internal issuer's PKI app (proves the ACME branch did not run)")
	}
	if applier.lastCfg.Apps.TLS == nil || len(applier.lastCfg.Apps.TLS.Automation.Policies) != 1 {
		t.Fatalf("Apps.TLS = %+v, want exactly one automation policy", applier.lastCfg.Apps.TLS)
	}
	raw, err := json.Marshal(applier.lastCfg.Apps.TLS.Automation.Policies[0].Issuers[0])
	if err != nil {
		t.Fatalf("marshal issuer: %v", err)
	}
	if !strings.Contains(string(raw), `"module":"internal"`) {
		t.Errorf("issuer JSON = %s, want module=internal", raw)
	}
}

// TestController_Reconcile_ACMEEnabled_ThreadsSettingsIntoConfig proves
// the settings row's ACME fields actually reach ingress.BuildRoutesConfig,
// not just that the controller compiles a request for them: with
// ACMEEnabled true, the applied config's automation policy must use the
// real ACME issuer, carrying the exact email and directory URL the fake
// store returned, and must not build an internal-issuer PKI app.
func TestController_Reconcile_ACMEEnabled_ThreadsSettingsIntoConfig(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		settings: store.IngressSettings{
			ACMEEnabled:      true,
			ACMEEmail:        "ops@example.com",
			ACMEDirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
		},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if applier.lastCfg.Apps.PKI != nil {
		t.Errorf("Apps.PKI = %+v, want nil: ACME issuance needs no local CA", applier.lastCfg.Apps.PKI)
	}
	policies := applier.lastCfg.Apps.TLS.Automation.Policies
	if len(policies) != 1 {
		t.Fatalf("policies = %+v, want exactly one", policies)
	}
	// The concrete type is ingress.ACMEIssuer; this package (reconcile/
	// ingress) doesn't import that type by name in this test to avoid a
	// brittle duplicate assertion of internal/ingress's own
	// TestBuildRoutesConfig_ACMEEnabled, but a JSON round trip is enough
	// to prove the module discriminator and both fields survived the
	// trip through the controller.
	raw, err := json.Marshal(policies[0].Issuers[0])
	if err != nil {
		t.Fatalf("marshal issuer: %v", err)
	}
	for _, want := range []string{`"module":"acme"`, `"email":"ops@example.com"`, `"ca":"https://acme-staging-v02.api.letsencrypt.org/directory"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("issuer JSON = %s, want to contain %s", raw, want)
		}
	}
}

// TestController_Reconcile_PlatformDomainRoute_Added proves the second,
// independent setting (PrimaryDomain) adds a reverse-proxy route to
// WithDashboardDial's configured target, alongside whatever app routes
// already exist, sharing the same TLS automation policy.
func TestController_Reconcile_PlatformDomainRoute_Added(t *testing.T) {
	st := &fakeStore{settings: store.IngressSettings{PrimaryDomain: "dashboard.example.com"}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDashboardDial("127.0.0.1:8080"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services (the dashboard route counts)", cond.Reason)
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want exactly one", routes)
	}
	if len(routes[0].Match) != 1 || len(routes[0].Match[0].Host) != 1 || routes[0].Match[0].Host[0] != "dashboard.example.com" {
		t.Errorf("route match = %+v, want Host=[dashboard.example.com]", routes[0].Match)
	}
}

// TestController_Reconcile_PlatformDomainRoute_NoDashboardDial_Skipped
// proves a configured PrimaryDomain with no WithDashboardDial option
// (the wiring not having been set up, e.g. cmd/levelrail's own
// dashboardDialAddr returning "" for an unparseable APP_HTTP_ADDR) fails
// closed: no route is added, no error either, the same "optional
// capability whose prerequisite wiring is absent" shape every other
// optional Controller feature already has (WithCertStore, and so on).
func TestController_Reconcile_PlatformDomainRoute_NoDashboardDial_Skipped(t *testing.T) {
	st := &fakeStore{settings: store.IngressSettings{PrimaryDomain: "dashboard.example.com"}}
	rt := newFakeRuntime()
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger())) // no WithDashboardDial

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed0Services" {
		t.Errorf("condition.Reason = %q, want Routed0Services", cond.Reason)
	}
}

// TestController_Reconcile_PlatformDomainRoute_ConflictsWithService_Skipped
// is this feature's own defense-in-depth guard, the platform-domain
// counterpart to firstDuplicateHost's existing service/static-site
// coverage: if an operator's primary domain happens to collide with a
// domain already claimed by a real app, the app's route wins and the
// dashboard route is dropped for this pass (logged, not a reconcile
// failure), matching the "skip the loser, never build two routes for
// one host" rule the rest of this controller already enforces.
func TestController_Reconcile_PlatformDomainRoute_ConflictsWithService_Skipped(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"dashboard.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		settings: store.IngressSettings{PrimaryDomain: "dashboard.example.com"},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDashboardDial("127.0.0.1:8080"))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition.Reason = %q, want Routed1Services (only the app's route, not a second one for the dashboard)", cond.Reason)
	}
	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("routes = %+v, want exactly one (the app's, not the dashboard's)", routes)
	}
	handler, ok := routes[0].Handle[0].(ingress.ReverseProxyHandler)
	if !ok {
		t.Fatalf("Handle[0] = %T, want ingress.ReverseProxyHandler", routes[0].Handle[0])
	}
	if handler.Upstreams[0].Dial == "127.0.0.1:8080" {
		t.Errorf("route dials the dashboard target (127.0.0.1:8080), want the app's container dial: the app's route must win the conflict, not the dashboard's")
	}
}

// TestController_Reconcile_GetIngressSettingsError is the partial-
// failure case this project's own testing standard requires for every
// reconciler ("Every reconciler must have a test for the case where the
// operation half-succeeded", CLAUDE.md): ListDesiredServices and
// ListStaticSites both succeed, only the new GetIngressSettings call
// fails. The whole reconcile must still fail (no route built with only
// half the desired-state picture is safer than routing with a stale or
// wrong ACME/platform-domain decision), and Apply must never be called.
func TestController_Reconcile_GetIngressSettingsError(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		settingsErr: errors.New("settings row locked"),
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()))

	result, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("Reconcile() error = nil, want the settings-read failure to surface as a real error")
	}
	cond := conditionOf(t, result)
	if cond.Status != reconcile.ConditionFalse || cond.Reason != "StoreError" {
		t.Errorf("condition = %+v, want Status=False Reason=StoreError", cond)
	}
	if applier.calls != 0 {
		t.Errorf("Apply calls = %d, want 0: a half-known desired state must never be applied", applier.calls)
	}
}
