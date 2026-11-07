package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cdn"
)

func TestAppEdge_ReportsProviderZoneAndLastPurge(t *testing.T) {
	rt, cookie := newCDNFixture(t, &cdnFake{ssl: "strict"}, true)
	if err := rt.purgeAppCacheFor(context.Background(), "web", cdn.TriggerDeploy); err != nil {
		t.Fatal(err)
	}
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/edge", "")
	var res edgeResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !res.Proxied || res.Provider != "cloudflare" || res.Zone != "example.com" || res.SSLMode != "strict" || res.Health != "ok" {
		t.Fatalf("res = %+v", res)
	}
	if res.LastPurge == nil || res.LastPurge.Trigger != cdn.TriggerDeploy || !res.LastPurge.OK {
		t.Fatalf("last purge = %+v", res.LastPurge)
	}
	if strings.Contains(rec.Body.String(), cdnTestToken) {
		t.Fatal("token in response")
	}
}

func TestSetupAppCDN_DryRunChangesNothingThenApplies(t *testing.T) {
	rt, cookie := newCDNFixture(t, &cdnFake{ssl: "strict"}, false)
	var dry cdn.SetupResult
	rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/cdn/setup", `{"dry_run":true}`)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &dry) != nil || dry.Applied {
		t.Fatalf("dry run %d: %s", rec.Code, rec.Body.String())
	}
	if cur, _ := rt.appCDNResource(context.Background(), "web"); cur.Effective {
		t.Fatal("dry run turned the proxy on")
	}
	var live cdn.SetupResult
	rec = doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/cdn/setup", `{}`)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &live) != nil || !live.Applied {
		t.Fatalf("apply %d: %s", rec.Code, rec.Body.String())
	}
	if cur, _ := rt.appCDNResource(context.Background(), "web"); !cur.Effective {
		t.Fatal("proxy not on after setup")
	}
}

func TestSetupAppCDN_FlexibleSSLBlocksApply(t *testing.T) {
	rt, cookie := newCDNFixture(t, &cdnFake{ssl: "flexible"}, false)
	var res cdn.SetupResult
	rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/cdn/setup", `{}`)
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if res.Status != cdn.StatusFail || res.Applied {
		t.Fatalf("res = %+v", res)
	}
}
