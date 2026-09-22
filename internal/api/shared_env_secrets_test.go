package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleSetSharedEnvSecret_NoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithSecretSetter
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/proj_1/env/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleSetSharedEnvSecret_UnknownProject_NotFound(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/proj_missing/env/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetSharedEnvSecret_EmptyValue_Rejected(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/proj_1/env/secrets/API_KEY", `{"value":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleSetSharedEnvSecret_Success covers the project/organization/
// environment tiers together: same handler plumbing
// (shared_env_secrets.go), only the seed resource and URL differ.
func TestHandleSetSharedEnvSecret_Success(t *testing.T) {
	tests := []struct {
		name       string
		seed       func(t *testing.T, db *store.DB)
		url        string
		wantNS     string
		listAllURL string
	}{
		{
			name: "project",
			seed: func(t *testing.T, db *store.DB) {
				t.Helper()
				if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
					t.Fatalf("SaveProject() error = %v", err)
				}
			},
			url:        "/api/v1/projects/proj_1/env/secrets/API_KEY",
			wantNS:     store.ProjectEnvSecretsKey("proj_1"),
			listAllURL: "/api/v1/projects/proj_1/env/all",
		},
		{
			name: "organization",
			seed: func(t *testing.T, db *store.DB) {
				t.Helper()
				if err := db.SaveOrganization(context.Background(), store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
					t.Fatalf("SaveOrganization() error = %v", err)
				}
			},
			url:        "/api/v1/organizations/org_1/env/secrets/API_KEY",
			wantNS:     store.OrganizationEnvSecretsKey("org_1"),
			listAllURL: "/api/v1/organizations/org_1/env/all",
		},
		{
			name: "environment",
			seed: func(t *testing.T, db *store.DB) {
				t.Helper()
				if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
					t.Fatalf("SaveProject() error = %v", err)
				}
				if err := db.SaveEnvironment(context.Background(), store.Environment{ID: "env_1", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
					t.Fatalf("SaveEnvironment() error = %v", err)
				}
			},
			url:        "/api/v1/environments/env_1/env/secrets/API_KEY",
			wantNS:     store.EnvironmentEnvSecretsKey("env_1"),
			listAllURL: "/api/v1/environments/env_1/env/all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setter := &fakeSecretSetter{}
			rt, db := newTestRouterWithSecrets(t, setter)
			cookie := loginTestSession(t, rt, db)
			tt.seed(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, tt.url, `{"value":"sk-abc"}`))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty: a secret value must never be echoed back", rec.Body.String())
			}
			if setter.calls != 1 || setter.lastService != tt.wantNS || setter.lastKey != "API_KEY" || setter.lastValue != "sk-abc" {
				t.Errorf("setter called with (%q, %q, %q), calls=%d, want (%q, API_KEY, sk-abc), calls=1",
					setter.lastService, setter.lastKey, setter.lastValue, setter.calls, tt.wantNS)
			}

			// GET .../env/all shows the key as secret-marked, with no value.
			rec2 := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, tt.listAllURL, ""))
			if rec2.Code != http.StatusOK {
				t.Fatalf("list all status = %d, want %d, body = %s", rec2.Code, http.StatusOK, rec2.Body.String())
			}
			var got []sharedEnvVarResource
			if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != 1 || got[0].Key != "API_KEY" || !got[0].Secret || got[0].Value != "" {
				t.Errorf("list all = %+v, want one secret-marked API_KEY entry with empty value", got)
			}
		})
	}
}

// TestHandleListSharedEnvAll_StaleFlag proves GET .../env/all flags a
// secret-marked row as stale once it's older than the configured
// rotation warning threshold, and never populates updated_at/stale for a
// plain (non-secret) row, matching sharedEnvVarResources' own doc
// comment.
func TestHandleListSharedEnvAll_StaleFlag(t *testing.T) {
	setter := &fakeSecretSetter{}
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db,
		WithSecretSetter(setter),
		WithSecretRotationWarnAge(24*time.Hour),
	)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SetProjectEnvVars(context.Background(), "proj_1", map[string]string{"LOG_LEVEL": "info"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/proj_1/env/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// Backdate the secret-marked row directly: 48h old against a 24h
	// configured threshold, the only way this test can simulate a stale
	// secret without sleeping for real.
	if _, err := db.ExecContext(context.Background(), `
		UPDATE project_env_vars SET updated_at = ? WHERE project_id = ? AND key = ?
	`, time.Now().Add(-48*time.Hour).UTC().Format("2006-01-02T15:04:05.000Z"), "proj_1", "API_KEY"); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_1/env/all", ""))
	if rec2.Code != http.StatusOK {
		t.Fatalf("list all status = %d, want %d, body = %s", rec2.Code, http.StatusOK, rec2.Body.String())
	}
	var got []sharedEnvVarResource
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byKey := map[string]sharedEnvVarResource{}
	for _, r := range got {
		byKey[r.Key] = r
	}
	if !byKey["API_KEY"].Stale {
		t.Errorf("API_KEY (48h old, 24h threshold) stale = false, want true")
	}
	if byKey["API_KEY"].UpdatedAt == "" {
		t.Errorf("API_KEY updated_at is empty, want a populated timestamp")
	}
	if byKey["LOG_LEVEL"].Stale {
		t.Errorf("LOG_LEVEL (a plain, non-secret var) stale = true, want false (never computed for plain rows)")
	}
	if byKey["LOG_LEVEL"].UpdatedAt != "" {
		t.Errorf("LOG_LEVEL updated_at = %q, want empty (never populated for plain rows)", byKey["LOG_LEVEL"].UpdatedAt)
	}
}

func TestHandleDeleteSharedEnvSecret_RemovesFromListAll(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/proj_1/env/secrets/API_KEY", `{"value":"sk-abc"}`))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodDelete, "/api/v1/projects/proj_1/env/secrets/API_KEY", ""))
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d, body = %s", rec2.Code, http.StatusNoContent, rec2.Body.String())
	}

	keys, err := db.ListProjectSecretEnvKeys(context.Background(), "proj_1")
	if err != nil {
		t.Fatalf("ListProjectSecretEnvKeys() error = %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("secret keys after delete = %v, want none", keys)
	}
}

func TestHandleDeleteSharedEnvSecret_NoSetterConfiguredStillWorks(t *testing.T) {
	// Deleting a shared env var secret's marker row needs no
	// Router.secrets: see handleDeleteSharedEnvSecret's own doc comment.
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SetProjectSecretEnvVar(context.Background(), "proj_1", "API_KEY"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/projects/proj_1/env/secrets/API_KEY", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleListSharedEnvSecretKeys_NeverEchoesAValue(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	rt.Handler().ServeHTTP(httptest.NewRecorder(), authedRequest(t, cookie, http.MethodPut, "/api/v1/projects/proj_1/env/secrets/API_KEY", `{"value":"sk-abc"}`))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_1/env/secrets", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "API_KEY") || strings.Contains(got, "sk-abc") {
		t.Errorf("body = %q, want it to list the key API_KEY and never the value sk-abc", got)
	}
}
