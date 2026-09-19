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

func TestHandleGetDomainRedirect_NoneConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/redirect", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainRedirectResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Enabled {
		t.Errorf("got = %+v, want disabled before anything is configured", got)
	}
}

func TestHandleGetDomainRedirect_DomainNotOwnedByApp(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/redirect", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for a domain the app does not own", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetDomainRedirect_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/domains/app.example.com/redirect", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDomainRedirect_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"target_url":"https://example.com","status_code":301}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainRedirectResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Enabled || got.TargetURL != "https://example.com" || got.StatusCode != 301 {
		t.Errorf("got = %+v, want Enabled=true TargetURL=https://example.com StatusCode=301", got)
	}

	row, found, err := db.GetDomainRedirect(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	}
	if !found || row.TargetURL != "https://example.com" || row.StatusCode != 301 {
		t.Errorf("GetDomainRedirect() = %+v, found=%v, want the same values persisted", row, found)
	}
}

func TestHandleSetDomainRedirect_DefaultsStatusCodeToPermanent(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"target_url":"https://example.com"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainRedirectResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StatusCode != store.DomainRedirectPermanent {
		t.Errorf("StatusCode = %d, want %d as the default when omitted", got.StatusCode, store.DomainRedirectPermanent)
	}
}

func TestHandleSetDomainRedirect_MissingTargetURL_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for a missing target_url", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetDomainRedirect_MalformedTargetURL_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	tests := []string{
		`{"target_url":"not-a-url"}`,
		`{"target_url":"/relative/path"}`,
		`{"target_url":"ftp://example.com"}`,
	}
	for _, body := range tests {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body = %s: status = %d, want %d for a malformed target_url", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleSetDomainRedirect_InvalidStatusCode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"target_url":"https://example.com","status_code":404}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for an invalid status_code", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleClearDomainRedirect_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", `{"target_url":"https://example.com"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/redirect", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if _, found, err := db.GetDomainRedirect(context.Background(), "app.example.com"); err != nil {
		t.Fatalf("GetDomainRedirect() error = %v", err)
	} else if found {
		t.Errorf("found = true, want false after clearing")
	}
}

func TestHandleClearDomainRedirect_Idempotent(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/redirect", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-cleared domain, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestDomainRedirectWriteRoutes_PlainWriteToken_Forbidden proves PUT/
// DELETE .../domains/{domain}/redirect sit behind AbilityDeploy, not
// just AbilityWrite, mirroring
// TestDomainWAFWriteRoutes_PlainWriteToken_Forbidden's identical shape
// for the sibling route.
func TestDomainRedirectWriteRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect", strings.NewReader(`{"target_url":"https://example.com"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the domain redirect write", rec.Code, http.StatusForbidden)
	}
}

func TestDomainRedirectRoutes_RequireAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/domains/app.example.com/redirect"},
		{http.MethodPut, "/api/v1/apps/web/domains/app.example.com/redirect"},
		{http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/redirect"},
	})
}
