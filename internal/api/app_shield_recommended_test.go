package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func recommendShield(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, shieldRecommendedResource) {
	t.Helper()
	rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/shield/recommended", body)
	var out shieldRecommendedResource
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func shieldRow(t *testing.T, db *store.DB, domain string) (trafficpolicy.Shield, bool) {
	t.Helper()
	row, found, err := db.GetDomainTrafficPolicy(context.Background(), domain, trafficpolicy.KindShield)
	if err != nil {
		t.Fatal(err)
	}
	var s trafficpolicy.Shield
	if found {
		if err := json.Unmarshal([]byte(row.Spec), &s); err != nil {
			t.Fatal(err)
		}
	}
	return s, found
}

func TestRecommendedShield_AppliesOnlyToUnconfiguredDomains(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "a.example.com", "b.example.com")
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	custom := []byte(`{"enabled":false,"rate_per_minute":5}`)
	if err := db.SetDomainTrafficPolicy(ctx, "b.example.com", trafficpolicy.KindShield, custom); err != nil {
		t.Fatal(err)
	}

	code, dry := recommendShield(t, rt, cookie, `{"dry_run":true}`)
	if code != http.StatusOK || len(dry.Applied) != 1 || dry.Applied[0] != "a.example.com" {
		t.Fatalf("dry run = %d %+v", code, dry)
	}
	if _, found := shieldRow(t, db, "a.example.com"); found {
		t.Fatal("a dry run wrote a shield")
	}

	code, res := recommendShield(t, rt, cookie, "")
	if code != http.StatusOK || len(res.Applied) != 1 || len(res.Skipped) != 1 || res.Skipped[0].Reason != shieldSkipConfigured {
		t.Fatalf("apply = %d %+v", code, res)
	}
	a, found := shieldRow(t, db, "a.example.com")
	if !found || !a.Enabled || a.RatePerMinute != trafficpolicy.DefaultLimits().ShieldRecommendedRate || a.BlockBots {
		t.Fatalf("a shield = %+v found=%v, want recommended limits and no bot blocking by default", a, found)
	}
	b, _ := shieldRow(t, db, "b.example.com")
	if b.Enabled || b.RatePerMinute != 5 {
		t.Fatalf("an existing shield was changed: %+v", b)
	}

	code, again := recommendShield(t, rt, cookie, `{"block_bots":true}`)
	if code != http.StatusOK || len(again.Applied) != 0 || len(again.Skipped) != 2 {
		t.Fatalf("second apply = %d %+v, want nothing applied", code, again)
	}
	if a2, _ := shieldRow(t, db, "a.example.com"); a2.BlockBots {
		t.Fatal("a repeat apply overwrote the first shield")
	}
}

func TestRecommendedShield_RefusesWhenProxyHidesClientIPs(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomains(t, db, "a.example.com")
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.UpdateIngressSettings(ctx, store.IngressSettings{DNSProxied: true}); err != nil {
		t.Fatal(err)
	}
	code, res := recommendShield(t, rt, cookie, "")
	if code != http.StatusOK || res.Blocked != shieldBlockedProxy || len(res.Applied) != 0 {
		t.Fatalf("behind proxy = %d %+v, want blocked", code, res)
	}
	if _, found := shieldRow(t, db, "a.example.com"); found {
		t.Fatal("a shield was written while client IPs are hidden")
	}

	rt.domainPolicy.getenv = func(k string) string {
		if k == "APP_INGRESS_TRUSTED_PROXIES" {
			return "10.0.0.0/8"
		}
		return ""
	}
	if _, res := recommendShield(t, rt, cookie, ""); res.Blocked != "" || len(res.Applied) != 1 {
		t.Fatalf("with trusted proxies = %+v, want applied", res)
	}
}

func TestRecommendedShield_AuthAndMissingApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if code, _ := recommendShield(t, rt, cookie, ""); code != http.StatusNotFound {
		t.Fatalf("missing app = %d, want 404", code)
	}
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodPost, "/api/v1/apps/web/shield/recommended"}})
	reader := storeUserWithAbilitiesForTest(t, db, "viewer@example.com", []string{AbilityRead})
	rec := doPolicy(t, rt, sessionCookieForTest(t, rt, reader.ID), http.MethodPost, "/api/v1/apps/web/shield/recommended", "")
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("read-only caller = %d, want 403", rec.Code)
	}
}
