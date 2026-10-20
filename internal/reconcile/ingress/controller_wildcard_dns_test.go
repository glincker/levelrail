package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDNSTokenResolver is a hand-written fake for
// CloudflareDNSTokenResolver, mirroring fakeApplier's "record what was
// asked for, return a scripted result" shape.
type fakeDNSTokenResolver struct {
	token       string
	err         error
	resolveArgs []string // serviceName, envKey from the last Resolve call
}

func (f *fakeDNSTokenResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.resolveArgs = []string{serviceName, envKey}
	if f.err != nil {
		return "", f.err
	}
	return f.token, nil
}

// TestController_Reconcile_CloudflareDNSDisabledByDefault_RegressionUnchanged
// is this feature's own regression test: WithCloudflareDNSTokens never
// set, or set but store.CloudflareDNSSettings.Enabled false, must produce
// the exact same single-policy config as if the feature didn't exist,
// even for a wildcard domain.
func TestController_Reconcile_CloudflareDNSDisabledByDefault_RegressionUnchanged(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		settings: store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		// dnsSettings left at zero value: Enabled false.
	}
	resolver := &fakeDNSTokenResolver{token: "should-not-be-used"}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithCloudflareDNSTokens(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	policies := applier.lastCfg.Apps.TLS.Automation.Policies
	if len(policies) != 1 {
		t.Fatalf("policies = %+v, want exactly one (DNS-01 must not activate while disabled)", policies)
	}
	raw, err := json.Marshal(policies[0].Issuers[0])
	if err != nil {
		t.Fatalf("marshal issuer: %v", err)
	}
	if strings.Contains(string(raw), "api_token") {
		t.Errorf("issuer JSON = %s, must not contain a cloudflare api_token while disabled", raw)
	}
}

// TestController_Reconcile_CloudflareDNSEnabled_ThreadsTokenIntoConfig
// proves the token actually reaches ingress.BuildRoutesConfig, split into
// its own DNS-01 policy, once both store.CloudflareDNSSettings.Enabled
// and WithCloudflareDNSTokens are set.
func TestController_Reconcile_CloudflareDNSEnabled_ThreadsTokenIntoConfig(t *testing.T) {
	wildcard := store.DesiredService{Name: "wild", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	plain := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}

	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(wildcard.Name, wildcard.Image, ""), 34567)
	rt.seedRunning(application.ContainerName(plain.Name, plain.Image, ""), 34568)

	st := &fakeStore{
		services:    []store.DesiredService{wildcard, plain},
		settings:    store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		dnsSettings: store.CloudflareDNSSettings{Enabled: true},
	}
	resolver := &fakeDNSTokenResolver{token: "cf-test-token"}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithCloudflareDNSTokens(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	wantArgs := []string{store.CloudflareDNSSecretsKey(), store.CloudflareDNSTokenEnvKey}
	if len(resolver.resolveArgs) != 2 || resolver.resolveArgs[0] != wantArgs[0] || resolver.resolveArgs[1] != wantArgs[1] {
		t.Errorf("Resolve() called with %v, want %v", resolver.resolveArgs, wantArgs)
	}

	policies := applier.lastCfg.Apps.TLS.Automation.Policies
	if len(policies) != 2 {
		t.Fatalf("policies = %+v, want exactly two (one DNS-01 for the wildcard, one plain ACME for the rest)", policies)
	}

	raw, err := json.Marshal(policies)
	if err != nil {
		t.Fatalf("marshal policies: %v", err)
	}
	for _, want := range []string{`"cf-test-token"`, `"*.example.com"`, `"web.example.com"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("policies JSON = %s, want to contain %s", raw, want)
		}
	}
}

// TestController_Reconcile_CloudflareDNSResolveError_FailsOpen proves a
// token-resolve failure never blocks the whole ingress reconcile: it
// just falls back to no DNS-01 for this pass.
func TestController_Reconcile_CloudflareDNSResolveError_FailsOpen(t *testing.T) {
	desired := store.DesiredService{Name: "wild", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:    []store.DesiredService{desired},
		settings:    store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		dnsSettings: store.CloudflareDNSSettings{Enabled: true},
	}
	resolver := &fakeDNSTokenResolver{err: errors.New("secrets: master key not set")}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithCloudflareDNSTokens(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: a token-resolve failure must not fail the whole reconcile", err)
	}

	policies := applier.lastCfg.Apps.TLS.Automation.Policies
	if len(policies) != 1 {
		t.Fatalf("policies = %+v, want exactly one (fell back to plain ACME)", policies)
	}
}
