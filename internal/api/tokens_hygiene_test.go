package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCreateToken_DefaultTTLSetting(t *testing.T) {
	cases := []struct {
		name    string
		setting string
		body    string
		wantMin time.Duration
		wantMax time.Duration
		none    bool
	}{
		{"setting off keeps the built-in 90 days", "0", `{"name":"a","abilities":["read"]}`, 89 * 24 * time.Hour, 91 * 24 * time.Hour, false},
		{"setting applies without a request expiry", "14", `{"name":"b","abilities":["read"]}`, 13 * 24 * time.Hour, 15 * 24 * time.Hour, false},
		{"explicit expiry wins", "14", `{"name":"c","abilities":["read"],"expires_in_days":3}`, 2 * 24 * time.Hour, 4 * 24 * time.Hour, false},
		{"explicit never wins", "14", `{"name":"d","abilities":["read"],"no_expiry":true}`, 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+runtimesettings.KeyTokensDefaultTTLDays, `{"value":"`+tc.setting+`"}`); rec.Code != http.StatusOK {
				t.Fatalf("set ttl = %d %s", rec.Code, rec.Body.String())
			}
			code, got := mintToken(t, rt, cookie, tc.body)
			if code != http.StatusCreated {
				t.Fatalf("status = %d", code)
			}
			if tc.none {
				if got.ExpiresAt != nil {
					t.Fatalf("expires_at = %v, want none", got.ExpiresAt)
				}
				return
			}
			if got.ExpiresAt == nil {
				t.Fatal("expires_at missing")
			}
			if d := time.Until(*got.ExpiresAt); d < tc.wantMin || d > tc.wantMax {
				t.Fatalf("lifetime = %s, want between %s and %s", d, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestUnusedTokens(t *testing.T) {
	now := time.Now()
	ago := func(days int) time.Time { return now.Add(-time.Duration(days) * 24 * time.Hour) }
	used := ago(2)
	revoked := ago(1)
	tokens := []store.APIToken{
		{ID: "old-never-used", CreatedAt: ago(40)},
		{ID: "old-used-long-ago", CreatedAt: ago(90), LastUsedAt: &[]time.Time{ago(45)}[0]},
		{ID: "recent-use", CreatedAt: ago(90), LastUsedAt: &used},
		{ID: "new", CreatedAt: ago(1)},
		{ID: "revoked", CreatedAt: ago(60), RevokedAt: &revoked},
		{ID: "expired", CreatedAt: ago(60), ExpiresAt: &[]time.Time{ago(5)}[0]},
	}
	got := unusedTokens(tokens, 30, now)
	if len(got) != 2 || got[0].ID != "old-never-used" || got[1].ID != "old-used-long-ago" {
		t.Fatalf("unused = %+v", got)
	}
	if off := unusedTokens(tokens, 0, now); len(off) != 0 {
		t.Fatalf("setting off listed %d tokens", len(off))
	}
}

func TestUnusedTokensRoute_NoSecretAndDoctorWarns(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if code, _ := mintToken(t, rt, cookie, `{"name":"fresh"}`); code != http.StatusCreated {
		t.Fatalf("mint = %d", code)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/tokens/unused", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var res unusedTokensResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.AlertDays != 0 || len(res.Items) != 0 {
		t.Fatalf("response = %+v, %v", res, err)
	}
	if r := rt.doctorCheckTokenHygiene(t.Context()); r.Status == doctorStatusUnknown {
		t.Fatalf("doctor check unknown: %s", r.Message)
	}
}

func TestMCPMintedTokenCannotMutate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	code, got := mintToken(t, rt, cookie, `{"name":"mcp-client","agent":{"name":"mcp"}}`)
	if code != http.StatusCreated {
		t.Fatalf("mint = %d", code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens/cleanup", nil)
	req.Header.Set("Authorization", "Bearer "+got.Token)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only token reached a mutating route: %d", rec.Code)
	}
}
