package ingress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDomainConds struct{ got []store.DomainCondition }

func (f *fakeDomainConds) ReplaceDomainConditions(_ context.Context, conds []store.DomainCondition) error {
	f.got = conds
	return nil
}

func (f *fakeDomainConds) find(domain, code string) (store.DomainCondition, bool) {
	for _, c := range f.got {
		if c.Domain == domain && c.Code == code {
			return c, true
		}
	}
	return store.DomainCondition{}, false
}

func TestDomainIssues_UnresolvedCustomCertIsDegradedNotSilent(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	st := &fakeStore{
		services: []store.DesiredService{desired},
		tlsCerts: []store.DomainTLSCert{{Domain: "web.example.com", UploadedAt: time.Now(), ExpiresAt: time.Now().AddDate(1, 0, 0)}},
	}
	rec := &fakeDomainConds{}
	c := New(st, rt, &fakeApplier{}, WithLogger(discardLogger()), WithDomainConditionsRecorder(rec),
		WithDomainTLSCertSecrets(&fakePEMResolver{err: errors.New("secret missing")}))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	got, ok := rec.find("web.example.com", store.DomainCondCustomCertUnresolved)
	if !ok || got.Severity != store.DomainConditionDegraded {
		t.Fatalf("conditions = %+v, want a degraded custom_cert_unresolved entry", rec.got)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("Ready = %+v, want True: a degraded domain is still routed", cond)
	}
}

func TestDomainIssues_DroppedClearsOnceFixed(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	st := &fakeStore{
		services:  []store.DesiredService{desired},
		basicAuth: []store.DomainBasicAuth{{Domain: "web.example.com", Username: "operator"}},
	}
	rec := &fakeDomainConds{}
	resolver := &fakePasswordResolver{err: errors.New("gone")}
	c := New(st, rt, &fakeApplier{}, WithLogger(discardLogger()), WithDomainConditionsRecorder(rec), WithDomainBasicAuthSecrets(resolver))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, ok := rec.find("web.example.com", store.DomainCondBasicAuthUnresolved); !ok || got.Severity != store.DomainConditionDropped {
		t.Fatalf("conditions = %+v, want a dropped basic_auth_unresolved entry", rec.got)
	}

	resolver.err = nil
	resolver.passwords = map[string]string{store.DomainBasicAuthSecretsKey("web.example.com"): "pw"}
	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(rec.got) != 0 {
		t.Errorf("conditions = %+v, want none after the password resolves again", rec.got)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("Ready = %+v, want True once fixed", cond)
	}
}

func TestDomainIssues_WildcardWithoutDNSProviderIsDegraded(t *testing.T) {
	desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	st := &fakeStore{services: []store.DesiredService{desired}, settings: store.IngressSettings{ACMEEnabled: true}}
	rec := &fakeDomainConds{}
	c := New(st, rt, &fakeApplier{}, WithLogger(discardLogger()), WithDomainConditionsRecorder(rec))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got, ok := rec.find("*.example.com", store.DomainCondDNS01Unavailable); !ok || got.Severity != store.DomainConditionDegraded {
		t.Fatalf("conditions = %+v, want a degraded dns01_unavailable entry", rec.got)
	}
}

// One host with a broken credential or a wildcard lacking DNS-01 must not
// take the app's healthy hosts out of the routing table.
func TestDomainIssues_BadHostDoesNotBreakSiblingHosts(t *testing.T) {
	desired := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"good.example.com", "locked.example.com", "*.wild.example.com"},
	}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
	st := &fakeStore{
		services:  []store.DesiredService{desired},
		settings:  store.IngressSettings{ACMEEnabled: true},
		basicAuth: []store.DomainBasicAuth{{Domain: "locked.example.com", Username: "operator"}},
	}
	rec := &fakeDomainConds{}
	applier := &fakeApplier{}
	c := New(st, rt, applier, WithLogger(discardLogger()), WithDomainConditionsRecorder(rec),
		WithDomainBasicAuthSecrets(&fakePasswordResolver{err: errors.New("gone")}))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Reason != "DomainsDropped" {
		t.Fatalf("Ready = %+v, want DomainsDropped surfaced, not a failed pass", cond)
	}
	if _, ok := rec.find("locked.example.com", store.DomainCondBasicAuthUnresolved); !ok {
		t.Errorf("conditions = %+v, want basic_auth_unresolved for the locked host", rec.got)
	}
	if _, ok := rec.find("*.wild.example.com", store.DomainCondDNS01Unavailable); !ok {
		t.Errorf("conditions = %+v, want dns01_unavailable for the wildcard host", rec.got)
	}
	routed := map[string]bool{}
	for _, r := range applier.routes(t) {
		for _, m := range r.Match {
			for _, h := range m.Host {
				routed[h] = true
			}
		}
	}
	if !routed["good.example.com"] {
		t.Errorf("routed hosts = %v, want good.example.com still routed", routed)
	}
}
