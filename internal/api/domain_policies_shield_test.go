package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestDomainPolicies_ShieldAndUnderAttack(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	const path = "/api/v1/apps/web/domains/app.example.com/shield"

	rec := doPolicy(t, rt, cookie, http.MethodGet, path, "")
	var got domainPolicyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || got.Configured {
		t.Fatalf("default GET = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, path, `{"enabled":true}`); rec.Code != 400 {
		t.Fatalf("empty enabled shield = %d, want 400", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPut, path, `{"enabled":true,"rate_per_minute":120,"block_bots":true,"allow":["10.0.0.0/8"]}`); rec.Code != 200 {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}

	if rec := doPolicy(t, rt, cookie, http.MethodPost, path+"/under-attack", `{"minutes":5}`); rec.Code != 200 {
		t.Fatalf("under-attack = %d %s", rec.Code, rec.Body.String())
	}
	row, found, err := db.GetDomainTrafficPolicy(context.Background(), "app.example.com", trafficpolicy.KindShield)
	if err != nil || !found {
		t.Fatalf("stored = %v %v", found, err)
	}
	var s trafficpolicy.Shield
	if err := json.Unmarshal([]byte(row.Spec), &s); err != nil || s.UnderAttackUntil == nil || !s.UnderAttack(time.Now()) || s.RatePerMinute != 120 {
		t.Fatalf("stored shield = %+v %v", s, err)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPost, path+"/under-attack", `{"minutes":999999}`); rec.Code != 400 {
		t.Fatalf("oversized under-attack = %d, want 400", rec.Code)
	}

	if rec := doPolicy(t, rt, cookie, http.MethodDelete, path+"/under-attack", ""); rec.Code != 200 {
		t.Fatalf("stand down = %d", rec.Code)
	}
	row, _, _ = db.GetDomainTrafficPolicy(context.Background(), "app.example.com", trafficpolicy.KindShield)
	s = trafficpolicy.Shield{}
	if err := json.Unmarshal([]byte(row.Spec), &s); err != nil || s.UnderAttackUntil != nil || !s.Enabled {
		t.Fatalf("after stand down = %+v %v", s, err)
	}

	if rec := doPolicy(t, rt, cookie, http.MethodDelete, path, ""); rec.Code != 200 {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPost, path+"/under-attack", ""); rec.Code != 200 {
		t.Fatalf("under-attack with nothing configured = %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPolicy(t, rt, cookie, http.MethodDelete, path+"/under-attack", ""); rec.Code != 200 {
		t.Fatalf("stand down = %d", rec.Code)
	}
	if _, found, _ := db.GetDomainTrafficPolicy(context.Background(), "app.example.com", trafficpolicy.KindShield); found {
		t.Fatal("stand down left an inert row")
	}
}

func TestShieldBotListEndpoint(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/system/shield/bots", "")
	var got shieldBotListResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || len(got.Agents) < 20 || got.Custom {
		t.Fatalf("bots = %d %s", rec.Code, rec.Body.String())
	}
}
