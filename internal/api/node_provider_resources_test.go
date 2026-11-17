package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDiscoveryProvisioner struct {
	fakeNodeProvisioner
	lists     atomic.Int32
	billings  atomic.Int32
	resources []provision.ProviderResource
	listErr   error
	balance   provision.SpendBalance
	invoice   provision.SpendInvoice
}

func (f *fakeDiscoveryProvisioner) ListResources(context.Context) ([]provision.ProviderResource, error) {
	f.lists.Add(1)
	return f.resources, f.listErr
}

func (f *fakeDiscoveryProvisioner) ReadBilling(context.Context) (provision.SpendBalance, provision.SpendInvoice) {
	f.billings.Add(1)
	return f.balance, f.invoice
}

func price(v float64) *float64 { return &v }

func newDiscoveryRouter(t *testing.T, f *fakeDiscoveryProvisioner) *Router {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("digitalocean"), store.NodeProviderTokenEnvKey, "tok")
	return NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), openTestDB(t), WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(func(string, string) (NodeProvisioner, error) { return f, nil }))
}

func discoveryGet(_ *Router, h http.HandlerFunc, suffix, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/node-providers/digitalocean/"+suffix+query, nil)
	req.SetPathValue("provider", "digitalocean")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestProviderResources_ManagedByTagAndProvisionRow(t *testing.T) {
	f := &fakeDiscoveryProvisioner{resources: []provision.ProviderResource{
		{ID: "11", Name: "tagged", Tags: []string{"levelrail"}, EstimatedMonthlyPrice: price(6)},
		{ID: "22", Name: "row", Tags: []string{}, PublicIPv4: "203.0.113.22"},
		{ID: "33", Name: "foreign", Tags: []string{"prod"}},
	}}
	rt := newDiscoveryRouter(t, f)
	now := time.Now().UTC()
	ctx := context.Background()
	_ = rt.nodeProvisions.SaveNodeProvision(ctx, store.NodeProvision{ID: "p1", Provider: "digitalocean", Name: "row", Status: "ready", ProviderServerID: "22", CreatedAt: now, UpdatedAt: now})
	rec := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	var got providerResourcesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Count != 3 || !got.Resources[0].ManagedByLevelrail || !got.Resources[1].ManagedByLevelrail || got.Resources[2].ManagedByLevelrail {
		t.Fatalf("managed flags wrong: %+v", got.Resources)
	}
	if got.Resources[0].EnrolledNode != nil {
		t.Errorf("tag only must not claim a node")
	}
}

func TestProviderResources_ProvisionRowMarksManaged(t *testing.T) {
	f := &fakeDiscoveryProvisioner{resources: []provision.ProviderResource{{ID: "22", Name: "row", Tags: []string{}}}}
	rt := newDiscoveryRouter(t, f)
	now := time.Now().UTC()
	ctx := context.Background()
	_ = rt.nodeProvisions.SaveNodeProvision(ctx, store.NodeProvision{ID: "p1", Provider: "digitalocean", Name: "row", Status: "ready", ProviderServerID: "22", NodeID: "n1", CreatedAt: now, UpdatedAt: now})
	// annotate resolves the node reference through the nodes list.
	list := rt.annotateResources(ctx, "digitalocean", f.resources)
	if !list[0].ManagedByLevelrail {
		t.Fatalf("row match must be managed: %+v", list[0])
	}
}

func TestProviderResources_CachedThenCooldown(t *testing.T) {
	f := &fakeDiscoveryProvisioner{resources: []provision.ProviderResource{}}
	rt := newDiscoveryRouter(t, f)
	if rec := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"resources":[]`) {
		t.Fatalf("empty account = %d %s", rec.Code, rec.Body)
	}
	rec := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "")
	if !strings.Contains(rec.Body.String(), `"cached":true`) || f.lists.Load() != 1 {
		t.Fatalf("second must be cached, lists=%d", f.lists.Load())
	}
	if rec := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "?refresh=1"); rec.Code != 200 || f.lists.Load() != 2 {
		t.Fatalf("refresh = %d lists=%d", rec.Code, f.lists.Load())
	}
	rec = discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "?refresh=1")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("cooldown = %d retry=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestProviderResources_ScopeMissingIs424WithoutProviderBody(t *testing.T) {
	f := &fakeDiscoveryProvisioner{listErr: fmt.Errorf("wrap: %w", &provision.ProviderError{Status: 403, Body: "secret-body"})}
	rt := newDiscoveryRouter(t, f)
	rec := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "")
	if rec.Code != http.StatusFailedDependency || !strings.Contains(rec.Body.String(), `"code":"provider_scope_missing"`) ||
		!strings.Contains(rec.Body.String(), `"needs":"droplet:read"`) || strings.Contains(rec.Body.String(), "secret-body") {
		t.Fatalf("scope = %d %s", rec.Code, rec.Body)
	}
}

func TestProviderResources_OtherErrorIs502AndUnsupportedIs501(t *testing.T) {
	f := &fakeDiscoveryProvisioner{listErr: errors.New("boom")}
	rt := newDiscoveryRouter(t, f)
	if rec := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d", rec.Code)
	}
	rt2, _ := newCapRouter(t)
	if rec := discoveryGet(rt2, rt2.handleListNodeProviderResources, "resources", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported = %d", rec.Code)
	}
}

func TestProviderSpend_AllGrantedCachedAndCooldown(t *testing.T) {
	asOf := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	f := &fakeDiscoveryProvisioner{
		resources: []provision.ProviderResource{{ID: "1", EstimatedMonthlyPrice: price(6)}, {ID: "2", EstimatedMonthlyPrice: price(24)}, {ID: "3"}},
		balance: provision.SpendBalance{SpendSection: provision.SpendSection{State: provision.SectionOK},
			MonthToDateUsage: &provision.SpendValue{Value: "11.21", Source: "digitalocean_balance", DataAsOf: &asOf}, GeneratedAt: &asOf},
		invoice: provision.SpendInvoice{SpendSection: provision.SpendSection{State: provision.SectionOK}, InvoiceID: "123"},
	}
	rt := newDiscoveryRouter(t, f)
	rec := discoveryGet(rt, rt.handleGetNodeProviderSpend, "spend", "")
	if rec.Code != 200 {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	var rep provision.SpendReport
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rep.Balance.MonthToDateUsage.Value != "11.21" || rep.Estimate.MonthlyTotal.Value != "30.00" || rep.Estimate.PricedCount != 2 || rep.Estimate.ResourceCount != 3 || rep.Estimate.MonthlyTotal.Source == "" || rep.Estimate.MonthlyTotal.DataAsOf == nil {
		t.Fatalf("report = %+v", rep)
	}
	rec = discoveryGet(rt, rt.handleGetNodeProviderSpend, "spend", "")
	if !strings.Contains(rec.Body.String(), `"cached":true`) || f.billings.Load() != 1 {
		t.Fatalf("second must be cached, billings=%d", f.billings.Load())
	}
	if rec := discoveryGet(rt, rt.handleGetNodeProviderSpend, "spend", "?refresh=1"); rec.Code != 200 || f.billings.Load() != 2 {
		t.Fatalf("refresh = %d billings=%d", rec.Code, f.billings.Load())
	}
	rec = discoveryGet(rt, rt.handleGetNodeProviderSpend, "spend", "?refresh=1")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("cooldown = %d", rec.Code)
	}
}

func TestProviderSpend_BillingAndDropletsScopeMissingPerSection(t *testing.T) {
	f := &fakeDiscoveryProvisioner{
		listErr: &provision.ProviderError{Status: 403, Body: "secret-body"},
		balance: provision.SpendBalance{SpendSection: provision.SpendSection{State: provision.SectionScopeMissing, Needs: "billing:read"}},
		invoice: provision.SpendInvoice{SpendSection: provision.SpendSection{State: provision.SectionScopeMissing, Needs: "billing:read"}},
	}
	rt := newDiscoveryRouter(t, f)
	rec := discoveryGet(rt, rt.handleGetNodeProviderSpend, "spend", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "secret-body") {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	var rep provision.SpendReport
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rep.Balance.State != provision.SectionScopeMissing || rep.Balance.Needs != "billing:read" ||
		rep.Estimate.State != provision.SectionScopeMissing || rep.Estimate.Needs != "droplet:read" || rep.Estimate.MonthlyTotal != nil {
		t.Fatalf("report = %+v", rep)
	}
}

type regionalDiscoveryProvisioner struct {
	fakeDiscoveryProvisioner
	regionsAsked []string
}

func (f *regionalDiscoveryProvisioner) ListServers(_ context.Context, region string) ([]provision.ProviderResource, error) {
	f.regionsAsked = append(f.regionsAsked, region)
	return []provision.ProviderResource{{ID: region + "/i-1", Region: region}}, nil
}

func TestProviderResources_RegionQueryIsValidatedAndPassed(t *testing.T) {
	f := &regionalDiscoveryProvisioner{}
	f.regions = []provision.Region{{ID: "eu-west-1", Name: "Ireland"}}
	rt := newDiscoveryRouter(t, &f.fakeDiscoveryProvisioner)
	rt.nodeProvisionerFactory = func(string, string) (NodeProvisioner, error) { return f, nil }

	bad := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "?region=mars-1")
	if bad.Code != http.StatusBadRequest || len(f.regionsAsked) != 0 {
		t.Fatalf("unknown region: code %d asked %v", bad.Code, f.regionsAsked)
	}
	ok := discoveryGet(rt, rt.handleListNodeProviderResources, "resources", "?region=eu-west-1")
	if ok.Code != http.StatusOK || len(f.regionsAsked) != 1 || f.regionsAsked[0] != "eu-west-1" || !strings.Contains(ok.Body.String(), "eu-west-1/i-1") {
		t.Fatalf("code %d asked %v body %s", ok.Code, f.regionsAsked, ok.Body.String())
	}
}
