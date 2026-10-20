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

func TestHandleGetDomainErrorPages_NoneConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/error-pages", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainErrorPagesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Pages) != 0 {
		t.Errorf("got.Pages = %+v, want empty before anything is configured", got.Pages)
	}
}

func TestHandleGetDomainErrorPages_DomainNotOwnedByApp(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/other.example.com/error-pages", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for a domain the app does not own", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetDomainErrorPages_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/domains/app.example.com/error-pages", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDomainErrorPage_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"status_code":404,"body":"<h1>not found</h1>"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainErrorPagesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Pages) != 1 || got.Pages[0].StatusCode != 404 || got.Pages[0].Body != "<h1>not found</h1>" {
		t.Errorf("got.Pages = %+v, want one 404 entry", got.Pages)
	}

	rows, err := db.ListDomainErrorPages(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(rows) != 1 || rows[0].StatusCode != 404 {
		t.Errorf("ListDomainErrorPages() = %+v, want the same value persisted", rows)
	}
}

func TestHandleSetDomainErrorPage_AddsSecondCodeAlongsideFirst(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", `{"status_code":404,"body":"a"}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", `{"status_code":503,"body":"b"}`))

	var got domainErrorPagesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Pages) != 2 {
		t.Errorf("got.Pages = %+v, want both 404 and 503 present", got.Pages)
	}
}

func TestHandleSetDomainErrorPage_InvalidStatusCode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"status_code":418,"body":"<h1>teapot</h1>"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for a status_code outside 404/500/502/503", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetDomainErrorPage_EmptyBody_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"status_code":404,"body":""}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for an empty body", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleClearDomainErrorPages_OneCode(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", `{"status_code":404,"body":"a"}`))
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", `{"status_code":503,"body":"b"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/error-pages?status_code=404", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rows, err := db.ListDomainErrorPages(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(rows) != 1 || rows[0].StatusCode != 503 {
		t.Errorf("ListDomainErrorPages() = %+v, want only the 503 mapping left", rows)
	}
}

func TestHandleClearDomainErrorPages_AllCodes(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", `{"status_code":404,"body":"a"}`))
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", `{"status_code":503,"body":"b"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/error-pages", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	rows, err := db.ListDomainErrorPages(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("ListDomainErrorPages() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("ListDomainErrorPages() = %+v, want none left", rows)
	}
}

func TestHandleClearDomainErrorPages_Idempotent(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/error-pages", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d on an already-cleared domain, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

// TestDomainErrorPagesWriteRoutes_PlainWriteToken_Forbidden proves
// PUT/DELETE .../domains/{domain}/error-pages sit behind AbilityDeploy,
// not just AbilityWrite, mirroring
// TestDomainWAFWriteRoutes_PlainWriteToken_Forbidden's identical shape
// for the sibling route.
func TestDomainErrorPagesWriteRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the domain error pages write", rec.Code, http.StatusForbidden)
	}
}

func TestDomainErrorPagesRoutes_RequireAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/domains/app.example.com/error-pages"},
		{http.MethodPut, "/api/v1/apps/web/domains/app.example.com/error-pages"},
		{http.MethodDelete, "/api/v1/apps/web/domains/app.example.com/error-pages"},
	})
}
