package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedTestEnvironmentForEnv(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "web", CreatedAt: "2026-08-25T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SaveEnvironment(context.Background(), store.Environment{
		ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-25T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}
}

func TestHandleSetEnvironmentEnv_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTestEnvironmentForEnv(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/environments/env_1/env", `{"LOG_LEVEL":"info"}`))
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

	stored, err := db.ListEnvironmentEnvVars(context.Background(), "env_1")
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars() error = %v", err)
	}
	if stored["LOG_LEVEL"] != "info" {
		t.Errorf("stored = %+v, want LOG_LEVEL=info", stored)
	}
}

func TestHandleSetEnvironmentEnv_UnknownEnvironment_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/environments/env_missing/env", `{"A":"1"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetEnvironmentEnv_MalformedBody_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTestEnvironmentForEnv(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/environments/env_1/env", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGetEnvironmentEnv_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedTestEnvironmentForEnv(t, db)
	if err := db.SetEnvironmentEnvVars(context.Background(), "env_1", map[string]string{"A": "1"}); err != nil {
		t.Fatalf("SetEnvironmentEnvVars() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/environments/env_1/env", ""))
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

func TestHandleGetEnvironmentEnv_UnknownEnvironment_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/environments/env_missing/env", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
