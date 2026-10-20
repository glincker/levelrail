package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleSetOrganizationEnv_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveOrganization(context.Background(), store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-08-24T00:00:00Z"}); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/organizations/org_1/env", `{"LOG_LEVEL":"info"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("response = %+v, want LOG_LEVEL=info", got)
	}

	stored, err := db.ListOrganizationEnvVars(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVars() error = %v", err)
	}
	if stored["LOG_LEVEL"] != "info" {
		t.Errorf("stored = %+v, want LOG_LEVEL=info", stored)
	}
}

func TestHandleSetOrganizationEnv_UnknownOrganization_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/organizations/org_missing/env", `{"A":"1"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetOrganizationEnv_MalformedBody_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveOrganization(context.Background(), store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-08-24T00:00:00Z"}); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/organizations/org_1/env", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGetOrganizationEnv_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveOrganization(context.Background(), store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-08-24T00:00:00Z"}); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}
	if err := db.SetOrganizationEnvVars(context.Background(), "org_1", map[string]string{"A": "1"}); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/organizations/org_1/env", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["A"] != "1" {
		t.Errorf("response = %+v, want A=1", got)
	}
}

func TestHandleGetOrganizationEnv_UnknownOrganization_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/organizations/org_missing/env", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
