package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppCache_AbilitiesAndPurgeHistory(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	const plaintext = "cache-read-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_cache_ro", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/api/v1/apps/web/cache", "", 200},
		{http.MethodGet, "/api/v1/apps/web/cache/purges", "", 200},
		{http.MethodGet, "/api/v1/system/cache", "", 200},
		{http.MethodPost, "/api/v1/apps/web/cache/purge", `{"scope":"all"}`, 403},
		{http.MethodPost, "/api/v1/apps/web/cache/preset", ``, 403},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+plaintext)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cdn/overview", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Fatalf("read token refused on overview: %d", rec.Code)
	}
}

func TestAppCache_PurgeScopesAndHistory(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	cases := map[string]int{
		`{"scope":"all"}`:                   200,
		`{"scope":"tag","value":"blog"}`:    200,
		`{"scope":"tag","value":"a b"}`:     400,
		`{"scope":"tag"}`:                   400,
		`{"scope":"prefix","value":"/a"}`:   200,
		`{"scope":"url","value":"nope"}`:    400,
		`{"scope":"nope"}`:                  400,
		`{"scope":"all","domain":"x.test"}`: 404,
	}
	for body, want := range cases {
		if rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/cache/purge", body); rec.Code != want {
			t.Errorf("purge %s = %d, want %d: %s", body, rec.Code, want, rec.Body.String())
		}
	}
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/cache/purges", "")
	var hist struct {
		Purges []purgeHistoryRow `json:"purges"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil || len(hist.Purges) != 3 {
		t.Fatalf("history = %d %s", rec.Code, rec.Body.String())
	}
	if hist.Purges[0].Actor == "" || hist.Purges[0].Domain != "app.example.com" {
		t.Fatalf("history row = %+v", hist.Purges[0])
	}
	if rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/domains/app.example.com/cache/purge", `{"scope":"tag","value":"x"}`); rec.Code != 200 {
		t.Fatalf("domain purge by tag = %d", rec.Code)
	}
	if rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/nope/cache/purges", ""); rec.Code != 404 {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}

func TestAppCache_PresetIsIdempotent(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	apply := func() presetDomainResult {
		rec := doPolicy(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/cache/preset", "")
		var out struct {
			Domains []presetDomainResult `json:"domains"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 || len(out.Domains) != 1 {
			t.Fatalf("preset = %d %s", rec.Code, rec.Body.String())
		}
		return out.Domains[0]
	}
	if first := apply(); !first.Applied || first.Rules != 1 {
		t.Fatalf("first = %+v", first)
	}
	if second := apply(); second.Applied || second.Rules != 1 {
		t.Fatalf("second = %+v", second)
	}
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/cache", "")
	var got appCacheResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Domains) != 1 || !got.Domains[0].Enabled || got.Limits.Location != cacheLocationIngress {
		t.Fatalf("app cache = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSystemCache_ListsApps(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	rec := doPolicy(t, rt, cookie, http.MethodGet, "/api/v1/system/cache", "")
	var got systemCacheResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Apps) != 1 || got.Apps[0].App != "web" || got.Limits.MaxBytes == 0 {
		t.Fatalf("system cache = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRankNodes(t *testing.T) {
	t.Parallel()
	per := map[string]*cdnNode{"": {Requests: 30, Apps: 2}, "n1": {ID: "n1", Requests: 70, Apps: 1}}
	out, labeled := rankNodes(per, []store.Node{{ID: "n1", Name: "edge-1", Region: "fsn1"}}, 100)
	if len(out) != 2 || out[0].Name != "edge-1" || out[0].Share != 0.7 || !labeled || out[1].ID != "" {
		t.Fatalf("rankNodes = %+v labeled=%v", out, labeled)
	}
}
