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

func TestHandleGetDomainWAF_NoneConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/waf", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainWAFResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.WAFEnabled || got.RateLimitEnabled {
		t.Errorf("got = %+v, want both disabled before anything is configured", got)
	}
	if got.WAFMode != store.DomainWAFModeDetect {
		t.Errorf("WAFMode = %q, want %q as the default before anything is configured", got.WAFMode, store.DomainWAFModeDetect)
	}
}

func TestHandleGetDomainWAF_DomainNotOwnedByApp(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/waf", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for a domain the app does not own", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetDomainWAF_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/domains/app.example.com/waf", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDomainWAF_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"waf_enabled":true,"waf_mode":"block","rate_limit_rps":10,"rate_limit_burst":20}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainWAFResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.WAFEnabled || got.WAFMode != "block" || !got.RateLimitEnabled || got.RateLimitRPS != 10 || got.RateLimitBurst != 20 {
		t.Errorf("got = %+v, want WAFEnabled=true WAFMode=block RateLimitEnabled=true RateLimitRPS=10 RateLimitBurst=20", got)
	}

	row, found, err := db.GetDomainWAF(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	}
	if !found || !row.WAFEnabled || row.WAFMode != "block" || row.RateLimitRPS != 10 {
		t.Errorf("GetDomainWAF() = %+v, found=%v, want the same values persisted", row, found)
	}
}

func TestHandleSetDomainWAF_DefaultsModeToDetect(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"waf_enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainWAFResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.WAFMode != store.DomainWAFModeDetect {
		t.Errorf("WAFMode = %q, want the safer detect-only default when omitted", got.WAFMode)
	}
}

func TestHandleSetDomainWAF_InvalidMode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"waf_enabled":true,"waf_mode":"delete-everything"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for an invalid waf_mode", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetDomainWAF_NegativeRateLimit_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"rate_limit_rps":-1}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for a negative rate_limit_rps", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleClearDomainWAF_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf", `{"waf_enabled":true}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/waf", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if _, found, err := db.GetDomainWAF(context.Background(), "app.example.com"); err != nil {
		t.Fatalf("GetDomainWAF() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after clearing")
	}
}

func TestHandleClearDomainWAF_Idempotent(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/waf", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-cleared domain, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestDomainWAFWriteRoutes_PlainWriteToken_Forbidden proves PUT/DELETE
// .../domains/{domain}/waf sit behind AbilityDeploy, not just
// AbilityWrite, mirroring
// TestDomainMaintenanceWriteRoutes_PlainWriteToken_Forbidden's identical
// shape for the sibling route.
func TestDomainWAFWriteRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the domain waf write", rec.Code, http.StatusForbidden)
	}
}

func TestDomainWAFRoutes_RequireAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/domains/app.example.com/waf"},
		{http.MethodPut, "/api/v1/apps/web/domains/app.example.com/waf"},
		{http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/waf"},
	})
}
