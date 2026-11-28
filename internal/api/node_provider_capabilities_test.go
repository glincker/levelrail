package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/store"
)

type fakeCapProvisioner struct {
	fakeNodeProvisioner
	probes atomic.Int32
}

func (f *fakeCapProvisioner) ProbeCapabilities(context.Context, int) provision.ProviderCapabilities {
	f.probes.Add(1)
	return provision.ProviderCapabilities{
		Provider: "digitalocean", Method: "probe",
		Capabilities: []provision.Capability{
			{ID: "billing.balance", State: provision.CapabilityMissing, Evidence: provision.EvidenceProbe403, Needs: "billing:read"},
			{ID: "instances.create", State: provision.CapabilityUnknown, Evidence: provision.EvidenceNotProbed, Needs: "droplet:create"},
		},
	}
}

func capRequest(t *testing.T, rt *Router, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/node-providers/digitalocean/capabilities"+query, nil)
	req.SetPathValue("provider", "digitalocean")
	rec := httptest.NewRecorder()
	rt.handleGetNodeProviderCapabilities(rec, req)
	return rec
}

func newCapRouter(t *testing.T) (*Router, *fakeCapProvisioner) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("digitalocean"), store.NodeProviderTokenEnvKey, "tok")
	fake := &fakeCapProvisioner{}
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), openTestDB(t), WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(func(string, string) (NodeProvisioner, error) { return fake, nil }))
	return rt, fake
}

func TestProviderCapabilities_CachedThenCooldown(t *testing.T) {
	rt, fake := newCapRouter(t)
	first := capRequest(t, rt, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first = %d %s", first.Code, first.Body)
	}
	var got provision.ProviderCapabilities
	_ = json.Unmarshal(first.Body.Bytes(), &got)
	if got.Cached || len(got.Capabilities) != 2 {
		t.Fatalf("first result = %+v", got)
	}
	second := capRequest(t, rt, "")
	_ = json.Unmarshal(second.Body.Bytes(), &got)
	if !got.Cached || fake.probes.Load() != 1 {
		t.Fatalf("second cached=%v probes=%d", got.Cached, fake.probes.Load())
	}
	if rec := capRequest(t, rt, "?refresh=1"); rec.Code != http.StatusOK || fake.probes.Load() != 2 {
		t.Fatalf("refresh = %d probes=%d", rec.Code, fake.probes.Load())
	}
	rec := capRequest(t, rt, "?refresh=1")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("cooldown = %d retry=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestProviderCapabilities_LearnedWriteAndNoProviderBody(t *testing.T) {
	rt, _ := newCapRouter(t)
	rt.recordProviderCapabilityLearned("digitalocean", "instances.create", &provision.ProviderError{Status: 403, Body: "secret-body"})
	rec := capRequest(t, rt, "")
	if strings.Contains(rec.Body.String(), "secret-body") {
		t.Fatal("provider body leaked")
	}
	var got provision.ProviderCapabilities
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	for _, c := range got.Capabilities {
		if c.ID == "instances.create" && (c.State != provision.CapabilityMissing || c.Evidence != provision.Evidence403Learned) {
			t.Fatalf("learned = %+v", c)
		}
	}
}

func TestProviderScopeMissingResponse(t *testing.T) {
	rt, _ := newCapRouter(t)
	rec := httptest.NewRecorder()
	if !rt.writeProviderScopeMissing(rec, "digitalocean", "billing.balance", &provision.ProviderError{Status: 403, Body: "raw"}) {
		t.Fatal("expected a scope error")
	}
	if !strings.Contains(rec.Body.String(), `"needs":"billing:read"`) || strings.Contains(rec.Body.String(), "raw") {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestProviderCapabilities_RecheckHonorsInterval(t *testing.T) {
	rt, fake := newCapRouter(t)
	ctx := context.Background()
	t.Setenv("APP_PROVISION_CAPABILITIES_RECHECK_INTERVAL_MINUTES", "0")
	rt.recheckProviderCapabilities(ctx)
	if fake.probes.Load() != 0 {
		t.Fatalf("interval 0 must disable the recheck, probes=%d", fake.probes.Load())
	}
	t.Setenv("APP_PROVISION_CAPABILITIES_RECHECK_INTERVAL_MINUTES", "5")
	rt.recheckProviderCapabilities(ctx)
	if fake.probes.Load() != 1 {
		t.Fatalf("never probed provider must be probed, probes=%d", fake.probes.Load())
	}
	rt.recheckProviderCapabilities(ctx)
	if fake.probes.Load() != 1 {
		t.Fatalf("fresh result must not be re-probed, probes=%d", fake.probes.Load())
	}
	e := rt.providerCaps.entry("digitalocean")
	e.probeMu.Lock()
	e.result.CheckedAt = time.Now().Add(-10 * time.Minute)
	e.probeMu.Unlock()
	rt.recheckProviderCapabilities(ctx)
	if fake.probes.Load() != 2 {
		t.Fatalf("stale result must be re-probed, probes=%d", fake.probes.Load())
	}
}

func TestProviderCapabilities_RecheckSkipsRunningProbe(t *testing.T) {
	rt, fake := newCapRouter(t)
	t.Setenv("APP_PROVISION_CAPABILITIES_RECHECK_INTERVAL_MINUTES", "5")
	e := rt.providerCaps.entry("digitalocean")
	e.probeMu.Lock()
	rt.recheckProviderCapabilities(context.Background())
	e.probeMu.Unlock()
	if fake.probes.Load() != 0 {
		t.Fatalf("a running probe must not be doubled, probes=%d", fake.probes.Load())
	}
}

func TestProviderCapabilities_RecheckTickerStopsWithContext(t *testing.T) {
	rt, _ := newCapRouter(t)
	t.Setenv("APP_PROVISION_CAPABILITIES_RECHECK_POLL_SECONDS", "1")
	ctx, cancel := context.WithCancel(context.Background())
	rt.StartProviderCapabilityRecheck(ctx)
	rt.StartProviderCapabilityRecheck(ctx)
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for rt.providerCaps.recheckActive.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rt.providerCaps.recheckActive.Load() {
		t.Fatal("ticker did not stop with its context")
	}
}
