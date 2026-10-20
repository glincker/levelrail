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

// fakeRoute53CredentialResolver is a hand-written fake for
// Route53DNSCredentialResolver, mirroring fakeDNSTokenResolver's own
// "record what was asked for, return a scripted result" shape, extended
// to a two-key credential pair keyed by envKey.
type fakeRoute53CredentialResolver struct {
	values      map[string]string
	err         error
	resolveArgs [][]string // one []string{serviceName, envKey} per Resolve call
}

func (f *fakeRoute53CredentialResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.resolveArgs = append(f.resolveArgs, []string{serviceName, envKey})
	if f.err != nil {
		return "", f.err
	}
	return f.values[envKey], nil
}

// TestController_Reconcile_Route53DNSDisabledByDefault_RegressionUnchanged
// mirrors TestController_Reconcile_CloudflareDNSDisabledByDefault_
// RegressionUnchanged exactly, for Route53: WithRoute53DNSCredentials
// never set, or set but store.Route53DNSSettings.Enabled false, must
// produce the exact same single-policy config as if the feature didn't
// exist, even for a wildcard domain.
func TestController_Reconcile_Route53DNSDisabledByDefault_RegressionUnchanged(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		settings: store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		// route53Settings left at zero value: Enabled false.
	}
	resolver := &fakeRoute53CredentialResolver{values: map[string]string{
		store.Route53DNSAccessKeyIDEnvKey:     "should-not-be-used",
		store.Route53DNSSecretAccessKeyEnvKey: "should-not-be-used",
	}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithRoute53DNSCredentials(resolver))

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
	if strings.Contains(string(raw), "access_key_id") {
		t.Errorf("issuer JSON = %s, must not contain a route53 access_key_id while disabled", raw)
	}
}

// TestController_Reconcile_Route53DNSEnabled_ThreadsCredentialsIntoConfig
// proves the credential pair actually reaches ingress.BuildRoutesConfig,
// split into its own DNS-01 policy, once both store.Route53DNSSettings.
// Enabled and WithRoute53DNSCredentials are set.
func TestController_Reconcile_Route53DNSEnabled_ThreadsCredentialsIntoConfig(t *testing.T) {
	wildcard := store.DesiredService{Name: "wild", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	plain := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}

	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(wildcard.Name, wildcard.Image, ""), 34567)
	rt.seedRunning(application.ContainerName(plain.Name, plain.Image, ""), 34568)

	st := &fakeStore{
		services:        []store.DesiredService{wildcard, plain},
		settings:        store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		route53Settings: store.Route53DNSSettings{Enabled: true, Region: "us-east-1", HostedZoneID: "Z123"},
	}
	resolver := &fakeRoute53CredentialResolver{values: map[string]string{
		store.Route53DNSAccessKeyIDEnvKey:     "AKIA-test",
		store.Route53DNSSecretAccessKeyEnvKey: "shh",
	}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithRoute53DNSCredentials(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(resolver.resolveArgs) != 2 {
		t.Fatalf("Resolve() called %d times, want 2 (access key id, secret access key)", len(resolver.resolveArgs))
	}
	for _, args := range resolver.resolveArgs {
		if args[0] != store.Route53DNSSecretsKey() {
			t.Errorf("Resolve() serviceName = %q, want %q", args[0], store.Route53DNSSecretsKey())
		}
	}

	policies := applier.lastCfg.Apps.TLS.Automation.Policies
	if len(policies) != 2 {
		t.Fatalf("policies = %+v, want exactly two (one DNS-01 for the wildcard, one plain ACME for the rest)", policies)
	}

	raw, err := json.Marshal(policies)
	if err != nil {
		t.Fatalf("marshal policies: %v", err)
	}
	for _, want := range []string{`"AKIA-test"`, `"shh"`, `"us-east-1"`, `"Z123"`, `"*.example.com"`, `"web.example.com"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("policies JSON = %s, want to contain %s", raw, want)
		}
	}
}

// TestController_Reconcile_Route53DNSResolveError_FailsOpen mirrors
// TestController_Reconcile_CloudflareDNSResolveError_FailsOpen exactly,
// for Route53: a credential-resolve failure never blocks the whole
// ingress reconcile, it just falls back to no DNS-01 for this pass.
func TestController_Reconcile_Route53DNSResolveError_FailsOpen(t *testing.T) {
	desired := store.DesiredService{Name: "wild", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:        []store.DesiredService{desired},
		settings:        store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		route53Settings: store.Route53DNSSettings{Enabled: true},
	}
	resolver := &fakeRoute53CredentialResolver{err: errors.New("secrets: master key not set")}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithRoute53DNSCredentials(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: a credential-resolve failure must not fail the whole reconcile", err)
	}

	policies := applier.lastCfg.Apps.TLS.Automation.Policies
	if len(policies) != 1 {
		t.Fatalf("policies = %+v, want exactly one (fell back to plain ACME)", policies)
	}
}

// TestController_Reconcile_BothDNSProvidersEnabled_CloudflarePrecedence
// proves the documented precedence rule: if both Cloudflare and Route53
// DNS-01 are enabled at once, Cloudflare wins, not an error and not both.
func TestController_Reconcile_BothDNSProvidersEnabled_CloudflarePrecedence(t *testing.T) {
	desired := store.DesiredService{Name: "wild", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services:        []store.DesiredService{desired},
		settings:        store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		dnsSettings:     store.CloudflareDNSSettings{Enabled: true},
		route53Settings: store.Route53DNSSettings{Enabled: true},
	}
	cfResolver := &fakeDNSTokenResolver{token: "cf-test-token"}
	r53Resolver := &fakeRoute53CredentialResolver{values: map[string]string{
		store.Route53DNSAccessKeyIDEnvKey:     "AKIA-should-not-be-used",
		store.Route53DNSSecretAccessKeyEnvKey: "should-not-be-used",
	}}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithCloudflareDNSTokens(cfResolver), WithRoute53DNSCredentials(r53Resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if len(r53Resolver.resolveArgs) != 0 {
		t.Errorf("Route53 resolver called %d times, want 0: cloudflare must take precedence without ever consulting route53", len(r53Resolver.resolveArgs))
	}

	raw, err := json.Marshal(applier.lastCfg.Apps.TLS.Automation.Policies)
	if err != nil {
		t.Fatalf("marshal policies: %v", err)
	}
	if !strings.Contains(string(raw), `"cf-test-token"`) {
		t.Errorf("policies JSON = %s, want the cloudflare token", raw)
	}
	if strings.Contains(string(raw), "access_key_id") {
		t.Errorf("policies JSON = %s, must not contain a route53 provider when cloudflare wins", raw)
	}
}
