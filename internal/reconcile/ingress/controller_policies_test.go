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

type policyFakeStore struct {
	*fakeStore
	rows []store.DomainTrafficPolicy
	err  error
}

func (p *policyFakeStore) ListDomainTrafficPolicies(context.Context) ([]store.DomainTrafficPolicy, error) {
	return p.rows, p.err
}

func policyFixture(t *testing.T, rows []store.DomainTrafficPolicy, listErr error) (*Controller, *fakeApplier) {
	t.Helper()
	web := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com", "b.example.com"}}
	api := store.DesiredService{Name: "api", Image: "api:v1", Port: 80, Domains: []string{"api.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 34001)
	rt.seedRunning(application.ContainerName(api.Name, api.Image, ""), 34002)
	st := &policyFakeStore{fakeStore: &fakeStore{services: []store.DesiredService{web, api}}, rows: rows, err: listErr}
	applier := &fakeApplier{}
	return New(st, rt, applier, WithLogger(discardLogger())), applier
}

func routeJSON(t *testing.T, applier *fakeApplier, host string) string {
	t.Helper()
	for _, r := range applier.routes(t) {
		if len(r.Match) > 0 && len(r.Match[0].Host) > 0 && r.Match[0].Host[0] == host {
			raw, _ := json.Marshal(r.Handle)
			return string(raw)
		}
	}
	t.Fatalf("no route for %s", host)
	return ""
}

func TestController_TrafficPolicies(t *testing.T) {
	rows := []store.DomainTrafficPolicy{
		{Domain: "a.example.com", Kind: "forwarders", Spec: []byte(`{"rules":[
			{"match":{"kind":"prefix","path":"/api"},"action":"app","app":"api","strip_prefix":true},
			{"match":{"kind":"prefix","path":"/meta"},"action":"url","url":"http://169.254.169.254/latest"}]}`)},
		{Domain: "a.example.com", Kind: "headers", Spec: []byte(`{"rules":[{"side":"response","op":"set","name":"X-Policy","value":"on"}]}`)},
		{Domain: "b.example.com", Kind: "cache", Spec: []byte(`{not json`)},
	}
	c, applier := policyFixture(t, rows, nil)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	a := routeJSON(t, applier, "a.example.com")
	for _, want := range []string{`"X-Policy":["on"]`, `"strip_path_prefix":"/api"`, `"dial":"127.0.0.1:34002"`, `"status_code":502`} {
		if !strings.Contains(a, want) {
			t.Errorf("a.example.com handlers missing %s:\n%s", want, a)
		}
	}
	if strings.Contains(a, "169.254.169.254") {
		t.Errorf("metadata address reached the config: %s", a)
	}
	if b := routeJSON(t, applier, "b.example.com"); strings.Contains(b, "levelrail_cache") || strings.Contains(b, "subroute") {
		t.Errorf("undecodable row must leave b.example.com on defaults, got %s", b)
	}
	if api := routeJSON(t, applier, "api.example.com"); strings.Contains(api, "X-Policy") {
		t.Errorf("policy leaked onto another domain: %s", api)
	}

	before := a
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if after := routeJSON(t, applier, "a.example.com"); after != before {
		t.Errorf("reconcile is not idempotent:\n%s\n%s", before, after)
	}
}

func TestController_TrafficPolicies_StoreErrorFailsPass(t *testing.T) {
	c, applier := policyFixture(t, nil, errors.New("disk on fire"))
	if _, err := c.Reconcile(context.Background()); err == nil {
		t.Fatal("Reconcile() succeeded although policies could not be read; geo and header rules would silently disappear")
	}
	if applier.calls != 0 {
		t.Fatal("config applied without the domain policies")
	}
}

func TestController_TrafficPolicies_StoreWithoutSupport(t *testing.T) {
	web := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"a.example.com"}}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(web.Name, web.Image, ""), 34001)
	applier := &fakeApplier{}
	c := New(&fakeStore{services: []store.DesiredService{web}}, rt, applier, WithLogger(discardLogger()))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}
