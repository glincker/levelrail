package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
)

const cdnTestToken = "cf-token-must-not-leak" //nolint:gosec // fake value used to assert it never leaks

type cdnFake struct {
	mu      sync.Mutex
	purged  [][]string
	ssl     string
	failing bool
}

func newCDNFixture(t *testing.T, f *cdnFake, proxied bool) (*Router, *http.Cookie) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": v}) }
		switch {
		case r.URL.Path == "/zones":
			ok([]map[string]string{{"id": "z1", "name": "example.com"}})
		case strings.HasSuffix(r.URL.Path, "/purge_cache"):
			if f.failing {
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errors": []map[string]string{{"message": "denied"}}})
				return
			}
			var b struct{ Hosts []string }
			_ = json.NewDecoder(r.Body).Decode(&b)
			f.mu.Lock()
			f.purged = append(f.purged, b.Hosts)
			f.mu.Unlock()
			ok(map[string]string{"id": "z1"})
		case strings.HasSuffix(r.URL.Path, "/settings/ssl"):
			ok(map[string]string{"value": f.ssl})
		}
	}))
	t.Cleanup(srv.Close)
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "img:v1", Port: 8080, Domains: []string{"app.example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppDNSProxied(context.Background(), "web", &proxied, "t"); err != nil {
		t.Fatal(err)
	}
	api := &dnsrecords.CloudflareAPI{Token: cdnTestToken, BaseURL: srv.URL}
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Provider: "cloudflare", Zone: "example.com.", Proxy: api}, nil
	}
	return rt, loginTestSession(t, rt, db)
}

func TestPurgeAppCache_PurgesProxiedDomains(t *testing.T) {
	f := &cdnFake{}
	rt, _ := newCDNFixture(t, f, true)
	if err := rt.purgeAppCache(context.Background(), "web"); err != nil {
		t.Fatal(err)
	}
	if len(f.purged) != 1 || len(f.purged[0]) != 1 || f.purged[0][0] != "app.example.com" {
		t.Fatalf("purged = %v", f.purged)
	}
}

func TestPurgeAppCache_SkipsUnproxiedAndOptOut(t *testing.T) {
	f := &cdnFake{}
	rt, _ := newCDNFixture(t, f, false)
	if err := rt.purgeAppCache(context.Background(), "web"); err != nil || len(f.purged) != 0 {
		t.Fatalf("unproxied purged %v err %v", f.purged, err)
	}
	f2 := &cdnFake{}
	rt2, _ := newCDNFixture(t, f2, true)
	t.Setenv(envCDNPurge, "off")
	if err := rt2.purgeAppCache(context.Background(), "web"); err != nil || len(f2.purged) != 0 {
		t.Fatalf("opt-out purged %v err %v", f2.purged, err)
	}
}

func TestPurgeAppCache_FailureIsAWarningWithoutToken(t *testing.T) {
	rt, _ := newCDNFixture(t, &cdnFake{failing: true}, true)
	warn := rt.purgeWarning(context.Background(), "web")
	if warn == "" || strings.Contains(warn, cdnTestToken) || !strings.Contains(warn, "denied") {
		t.Fatalf("warning = %q", warn)
	}
}

func TestAppCDN_ReportsSSLMode(t *testing.T) {
	rt, cookie := newCDNFixture(t, &cdnFake{ssl: "flexible"}, true)
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/cdn", "")
	var res appCDNResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if res.SSLMode != "flexible" || !strings.Contains(res.SSLWarning, "Full (strict)") {
		t.Fatalf("res = %+v", res)
	}
	if strings.Contains(rec.Body.String(), cdnTestToken) {
		t.Fatal("token in response")
	}
}
