package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/store"
)

// scopeFake fails the catalog reads with a provider error and otherwise
// behaves like the shared fake.
type scopeFake struct {
	fakeNodeProvisioner
	catalogErr error
}

func (f *scopeFake) ListRegions(ctx context.Context) ([]provision.Region, error) {
	if f.catalogErr != nil {
		return nil, f.catalogErr
	}
	return f.fakeNodeProvisioner.ListRegions(ctx)
}

func (f *scopeFake) ListSizes(ctx context.Context, region string) ([]provision.Size, error) {
	if f.catalogErr != nil {
		return nil, f.catalogErr
	}
	return f.fakeNodeProvisioner.ListSizes(ctx, region)
}

func scopeForbidden() error {
	return &provision.ProviderError{Status: http.StatusForbidden, Body: "raw-provider-body"}
}

func assertScopeBody(t *testing.T, rec *httptest.ResponseRecorder, needs string) {
	t.Helper()
	if rec.Code != http.StatusFailedDependency {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["code"] != "provider_scope_missing" || got["needs"] != needs {
		t.Fatalf("body = %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), "raw-provider-body") {
		t.Fatal("provider body leaked")
	}
}

func scopeRouter(t *testing.T, fake NodeProvisioner) (*Router, *http.Cookie, *store.DB) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("digitalocean"), store.NodeProviderTokenEnvKey, "tok")
	rt, db := newTestRouterWithNodeProvisioningAny(t, secrets, fake)
	return rt, loginTestSession(t, rt, db), db
}

func newTestRouterWithNodeProvisioningAny(t *testing.T, secrets NodeProviderSecrets, fake NodeProvisioner) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(func(string, string) (NodeProvisioner, error) { return fake, nil }),
		WithAgentCAFingerprint("test-ca-fingerprint"))
	return rt, db
}

func TestProviderScope_RegionsAndSizes(t *testing.T) {
	rt, cookie, _ := scopeRouter(t, &scopeFake{catalogErr: scopeForbidden()})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-providers/digitalocean/regions", ""))
	assertScopeBody(t, rec, "regions:read")
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/node-providers/digitalocean/sizes?region=nyc3", ""))
	assertScopeBody(t, rec, "sizes:read")
}

func learnedState(t *testing.T, rt *Router, id string) (string, string) {
	t.Helper()
	res := &provision.ProviderCapabilities{Capabilities: []provision.Capability{{ID: id, State: provision.CapabilityUnknown, Evidence: provision.EvidenceNotProbed}}}
	overlayLearned(res, rt.providerCaps.entry("digitalocean").learned)
	return res.Capabilities[0].State, res.Capabilities[0].Evidence
}

func TestProviderScope_CreateLearnsAndFails(t *testing.T) {
	fake := &scopeFake{fakeNodeProvisioner: fakeNodeProvisioner{createErr: scopeForbidden()}}
	rt, cookie, _ := scopeRouter(t, fake)
	body := `{"provider":"digitalocean","region":"nyc3","size":"s","name":"web-1","role":"general","control_plane_addr":"cp.example.com:9443"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/provision", body))
	assertScopeBody(t, rec, "droplet:create")
	if st, ev := learnedState(t, rt, "instances.create"); st != provision.CapabilityMissing || ev != provision.Evidence403Learned {
		t.Fatalf("learned = %s %s", st, ev)
	}

	ok := &scopeFake{fakeNodeProvisioner: fakeNodeProvisioner{createServerID: "1", createIPAddr: "1.2.3.4"}}
	rt2, cookie2, _ := scopeRouter(t, ok)
	body = strings.Replace(body, "web-1", "web-2", 1)
	rec = httptest.NewRecorder()
	rt2.Handler().ServeHTTP(rec, authedRequest(t, cookie2, http.MethodPost, "/api/v1/nodes/provision", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if st, ev := learnedState(t, rt2, "instances.create"); st != provision.CapabilityGranted || ev != provision.EvidenceLearnedOK {
		t.Fatalf("learned = %s %s", st, ev)
	}
}

func TestProviderScope_DestroyLearnsAndFails(t *testing.T) {
	fake := &scopeFake{fakeNodeProvisioner: fakeNodeProvisioner{deleteErr: scopeForbidden()}}
	rt, cookie, db := scopeRouter(t, fake)
	now := time.Now()
	_ = db.SaveNodeProvision(context.Background(), store.NodeProvision{
		ID: "npv_s1", Provider: "digitalocean", Name: "web-s1", Role: "general",
		Status: store.NodeProvisionStatusBooting, ProviderServerID: "srv-1", CreatedAt: now, UpdatedAt: now,
	})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/node-provisions/npv_s1", ""))
	assertScopeBody(t, rec, "droplet:delete")
	if st, _ := learnedState(t, rt, "instances.delete"); st != provision.CapabilityMissing {
		t.Fatalf("learned = %s", st)
	}
	fake.deleteErr = nil
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/node-provisions/npv_s1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("destroy retry = %d %s", rec.Code, rec.Body)
	}
	if st, ev := learnedState(t, rt, "instances.delete"); st != provision.CapabilityGranted || ev != provision.EvidenceLearnedOK {
		t.Fatalf("learned = %s %s", st, ev)
	}
}

func TestProviderScope_ConnectionTestCarriesNeeds(t *testing.T) {
	rt, cookie, _ := scopeRouter(t, &scopeFake{catalogErr: scopeForbidden()})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/node-providers/digitalocean/test", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var got nodeProviderTestResult
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.OK || got.Code != "provider_scope_missing" || got.Needs != "regions:read" || strings.Contains(rec.Body.String(), "raw-provider-body") {
		t.Fatalf("result = %s", rec.Body)
	}
}

func TestProviderCapabilities_SaveAndTestResponses(t *testing.T) {
	fake := &fakeCapProvisioner{}
	rt, cookie, _ := scopeRouter(t, fake)
	post := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
		return rec
	}
	rec := post("/api/v1/node-providers", `{"provider":"digitalocean","token":"candidate-token"}`)
	var saved nodeProviderResource
	_ = json.Unmarshal(rec.Body.Bytes(), &saved)
	if rec.Code != http.StatusOK || saved.Capabilities == nil || len(saved.Capabilities.Capabilities) != 2 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "candidate-token") {
		t.Fatal("token echoed")
	}

	rec = post("/api/v1/node-providers/digitalocean/test", "")
	var stored nodeProviderTestResult
	_ = json.Unmarshal(rec.Body.Bytes(), &stored)
	if stored.Capabilities == nil || fake.probes.Load() != 2 {
		t.Fatalf("stored test = %s probes=%d", rec.Body, fake.probes.Load())
	}

	rec = post("/api/v1/node-providers/digitalocean/test", `{"provider":"digitalocean","token":"other"}`)
	var cand nodeProviderTestResult
	_ = json.Unmarshal(rec.Body.Bytes(), &cand)
	if cand.Capabilities == nil || fake.probes.Load() != 3 {
		t.Fatalf("candidate test = %s probes=%d", rec.Body, fake.probes.Load())
	}
	// A candidate probe must not replace the cached result of the stored credential.
	if e := rt.providerCaps.entry("digitalocean"); e.result == nil {
		t.Fatal("stored cache lost")
	}
}
