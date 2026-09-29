package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppConnectionsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/connections"},
		{http.MethodPost, "/api/v1/apps/web/connections"},
		{http.MethodDelete, "/api/v1/apps/web/connections/DATABASE_URL"},
		{http.MethodGet, "/api/v1/apps/web/connectable-databases"},
	})
}

func TestHandleCreateAppConnection_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedRedisDatabaseForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/missing/connections", `{"database":"main"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleCreateAppConnection_MissingDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateAppConnection_UnknownDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"missing"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateAppConnection_UnsupportedField(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	seedRedisDatabaseForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"main","field":"username"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateAppConnection_DefaultEnvVar(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got appConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.EnvVar != "MAIN_DATABASE_URL" {
		t.Fatalf("env_var = %q, want %q", got.EnvVar, "MAIN_DATABASE_URL")
	}
	if got.Field != "url" || got.DatabaseName != "main" {
		t.Fatalf("unexpected resource: %+v", got)
	}
	// No mesh configured in tests, so this must fall back to the
	// database's container name, not a mesh DNS name.
	if got.MeshDNS {
		t.Fatalf("mesh_dns = true, want false with no mesh configured")
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if ref, ok := svc.DatabaseEnv["MAIN_DATABASE_URL"]; !ok || ref.Database != "main" || ref.Field != "url" {
		t.Fatalf("DatabaseEnv not persisted correctly: %+v", svc.DatabaseEnv)
	}
}

// TestHandleCreateAppConnection_DefaultEnvVarCollision covers the
// collision-safe suffixing defaultConnectionEnvVar's own doc comment
// promises: a second connection to the same database+field must not
// silently overwrite the first's env var.
func TestHandleCreateAppConnection_DefaultEnvVarCollision(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"main"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("iteration %d: status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if len(svc.DatabaseEnv) != 2 {
		t.Fatalf("DatabaseEnv = %+v, want 2 distinct entries", svc.DatabaseEnv)
	}
	if _, ok := svc.DatabaseEnv["MAIN_DATABASE_URL"]; !ok {
		t.Fatalf("expected MAIN_DATABASE_URL, got %+v", svc.DatabaseEnv)
	}
	if _, ok := svc.DatabaseEnv["MAIN_DATABASE_URL_2"]; !ok {
		t.Fatalf("expected MAIN_DATABASE_URL_2, got %+v", svc.DatabaseEnv)
	}
}

func TestHandleCreateAppConnection_CustomEnvVar(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/connections", `{"database":"main","field":"host","env_var":"PRIMARY_DB_HOST"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got appConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.EnvVar != "PRIMARY_DB_HOST" || got.Field != "host" {
		t.Fatalf("unexpected resource: %+v", got)
	}
}

func TestHandleDeleteAppConnection(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.SetServiceDatabaseEnvVar(context.Background(), "web", "DATABASE_URL", &store.DatabaseEnvRef{Database: "main", Field: "url"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/connections/DATABASE_URL", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if _, ok := svc.DatabaseEnv["DATABASE_URL"]; ok {
		t.Fatalf("DatabaseEnv still has DATABASE_URL after delete: %+v", svc.DatabaseEnv)
	}
}

func TestHandleDeleteAppConnection_Idempotent(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/connections/NEVER_SET", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleListAppConnections(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.SetServiceDatabaseEnvVar(context.Background(), "web", "DATABASE_URL", &store.DatabaseEnvRef{Database: "main", Field: "url"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/connections", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []appConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].EnvVar != "DATABASE_URL" || got[0].DatabaseName != "main" {
		t.Fatalf("unexpected list: %+v", got)
	}
}

// TestHandleListAppConnections_SkipsDeletedDatabase covers the "one
// entry's problem, not the caller's" tolerance this handler's own doc
// comment promises: a DatabaseEnv entry referencing a since-deleted
// database is skipped, not a 500 for the whole list.
func TestHandleListAppConnections_SkipsDeletedDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SetServiceDatabaseEnvVar(context.Background(), "web", "DATABASE_URL", &store.DatabaseEnvRef{Database: "gone", Field: "url"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/connections", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []appConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty (referenced database no longer exists)", got)
	}
}

func TestHandleListConnectableDatabases(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.SetServiceDatabaseEnvVar(context.Background(), "web", "DATABASE_URL", &store.DatabaseEnvRef{Database: "main", Field: "url"}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/connectable-databases", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []connectableDatabaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	byName := map[string]connectableDatabaseResource{}
	for _, d := range got {
		byName[d.Name] = d
	}
	if !byName["main"].AlreadyConnected {
		t.Fatalf("main should be marked already_connected: %+v", byName["main"])
	}
	if len(byName["main"].ConnectedEnvVars) != 1 || byName["main"].ConnectedEnvVars[0] != "DATABASE_URL" {
		t.Fatalf("main connected_env_vars = %+v, want [DATABASE_URL]", byName["main"].ConnectedEnvVars)
	}
	if byName["cache"].AlreadyConnected {
		t.Fatalf("cache should not be marked already_connected: %+v", byName["cache"])
	}
	// Both app and databases default to the local node ("") in this
	// test, so neither is cross-node.
	if byName["main"].CrossNode || byName["cache"].CrossNode {
		t.Fatalf("unexpected cross_node with everything on the local node: %+v", got)
	}
}

func TestHandleListConnectableDatabases_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/connectable-databases", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestDefaultConnectionEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		database string
		field    string
		existing map[string]bool
		want     string
	}{
		{"url field", "main", "url", nil, "MAIN_DATABASE_URL"},
		{"host field", "main", "host", nil, "MAIN_DB_HOST"},
		{"non-alnum database name", "my-db.01", "url", nil, "MY_DB_01_DATABASE_URL"},
		{"collision appends suffix", "main", "url", map[string]bool{"MAIN_DATABASE_URL": true}, "MAIN_DATABASE_URL_2"},
		{"two collisions", "main", "url", map[string]bool{"MAIN_DATABASE_URL": true, "MAIN_DATABASE_URL_2": true}, "MAIN_DATABASE_URL_3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultConnectionEnvVar(tt.database, tt.field, tt.existing)
			if got != tt.want {
				t.Fatalf("defaultConnectionEnvVar(%q, %q, %v) = %q, want %q", tt.database, tt.field, tt.existing, got, tt.want)
			}
		})
	}
}
