package ingress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakePEMResolver is a hand-written fake for DomainTLSCertPEMResolver,
// mirroring fakePasswordResolver's own "record what was asked for,
// return a scripted result" shape.
type fakePEMResolver struct {
	values      map[string]string // keyed by serviceName+"/"+envKey
	err         error
	resolveArgs [][2]string
}

func (f *fakePEMResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	f.resolveArgs = append(f.resolveArgs, [2]string{serviceName, envKey})
	if f.err != nil {
		return "", f.err
	}
	return f.values[serviceName+"/"+envKey], nil
}

func newFakePEMResolverFor(domain, certPEM, keyPEM string) *fakePEMResolver {
	key := store.DomainTLSCertSecretsKey(domain)
	return &fakePEMResolver{values: map[string]string{
		key + "/" + store.DomainTLSCertCertificateEnvKey: certPEM,
		key + "/" + store.DomainTLSCertPrivateKeyEnvKey:  keyPEM,
	}}
}

// TestController_Reconcile_DomainTLSCert_NoResolverConfigured_RegressionUnchanged
// is this feature's own regression test: a domain_tls_cert row exists
// but WithDomainTLSCertSecrets was never called, the same "feature wired
// at the store layer but not enforced" state a control plane started
// without a master key is in. Unlike basic auth, this must not remove
// the domain's route: the host still gets Caddy's normal automatic
// issuance, it just doesn't get the BYO certificate.
func TestController_Reconcile_DomainTLSCert_NoResolverConfigured_RegressionUnchanged(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		tlsCerts: []store.DomainTLSCert{{Domain: "web.example.com", UploadedAt: time.Now(), ExpiresAt: time.Now().AddDate(1, 0, 0)}},
	}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger())) // no WithDomainTLSCertSecrets

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition reason = %q, want Routed1Services: the domain must still route normally without a resolver", cond.Reason)
	}
	if routes := applier.routes(t); len(routes) != 1 {
		t.Errorf("applied routes = %+v, want exactly one (normal issuance, no BYO cert)", routes)
	}
}

// TestController_Reconcile_DomainTLSCert_Configured_LoadsPEMCertificate
// proves a domain_tls_cert row plus a working resolver produces a
// tls.certificates.load_pem entry carrying the resolved cert/key PEM,
// while the route itself is unaffected (BYO TLS changes how a host's
// certificate is sourced, not how it's routed).
func TestController_Reconcile_DomainTLSCert_Configured_LoadsPEMCertificate(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		tlsCerts: []store.DomainTLSCert{{Domain: "web.example.com", UploadedAt: time.Now(), ExpiresAt: time.Now().AddDate(1, 0, 0)}},
	}
	resolver := newFakePEMResolverFor("web.example.com", "cert-pem", "key-pem")
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainTLSCertSecrets(resolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition reason = %q, want Routed1Services", cond.Reason)
	}

	if len(resolver.resolveArgs) != 2 {
		t.Fatalf("Resolve() called %d times, want exactly 2 (certificate, then private key)", len(resolver.resolveArgs))
	}

	cfg := applier.lastCfg
	if cfg == nil || cfg.Apps.TLS == nil || cfg.Apps.TLS.Certificates == nil {
		t.Fatalf("cfg.Apps.TLS.Certificates missing, want a load_pem entry")
	}
	pairs, ok := cfg.Apps.TLS.Certificates["load_pem"].([]ingress.CertKeyPEMPair)
	if !ok || len(pairs) != 1 {
		t.Fatalf("load_pem = %+v, want exactly one pair", cfg.Apps.TLS.Certificates["load_pem"])
	}
	if pairs[0].CertificatePEM != "cert-pem" || pairs[0].KeyPEM != "key-pem" {
		t.Errorf("pair = %+v, want CertificatePEM=cert-pem KeyPEM=key-pem", pairs[0])
	}

	routes := applier.routes(t)
	if len(routes) != 1 {
		t.Fatalf("applied routes = %+v, want exactly 1: BYO TLS must not change routing", routes)
	}
}

// TestController_Reconcile_DomainTLSCert_ResolveError_FallsBackToAutomaticIssuance
// proves a certificate-resolve failure never fails the whole reconcile
// and never drops the domain's route: it just falls back to Caddy's
// normal automatic issuance for that host, the deliberate fail-open
// choice WithDomainTLSCertSecrets documents (unlike basic auth's
// fail-closed one, an unloadable certificate is an availability problem,
// not a security control).
func TestController_Reconcile_DomainTLSCert_ResolveError_FallsBackToAutomaticIssuance(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	target := application.ContainerName(desired.Name, desired.Image, "")

	rt := newFakeRuntime()
	rt.seedRunning(target, 34567)

	st := &fakeStore{
		services: []store.DesiredService{desired},
		tlsCerts: []store.DomainTLSCert{{Domain: "web.example.com", UploadedAt: time.Now(), ExpiresAt: time.Now().AddDate(1, 0, 0)}},
	}
	resolver := &fakePEMResolver{err: errors.New("secrets: master key not set")}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainTLSCertSecrets(resolver))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v, want nil: a certificate-resolve failure must not fail the whole reconcile", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "Routed1Services" {
		t.Errorf("condition reason = %q, want Routed1Services: the domain must still route, just without the BYO cert", cond.Reason)
	}

	cfg := applier.lastCfg
	if cfg != nil && cfg.Apps.TLS != nil && cfg.Apps.TLS.Certificates != nil {
		t.Errorf("cfg.Apps.TLS.Certificates = %+v, want none: resolve failed, so no load_pem entry", cfg.Apps.TLS.Certificates)
	}
}

// TestController_Reconcile_DomainTLSCert_StoreListError_FailsReconcile
// proves a genuine store failure (not a secrets-resolve failure) does
// fail the whole reconcile, the same "only a real listing failure is an
// error" shape every other ServiceStore method already has.
func TestController_Reconcile_DomainTLSCert_StoreListError_FailsReconcile(t *testing.T) {
	st := &fakeStore{tlsCertsErr: errors.New("db: connection lost")}
	applier := &fakeApplier{}
	c := New(st, newFakeRuntime(), applier, WithLogger(discardLogger()))

	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() error = nil, want an error when ListDomainTLSCerts fails")
	}
}
