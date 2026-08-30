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

func TestValidateDatabaseResource(t *testing.T) {
	tests := []struct {
		name    string
		db      databaseResource
		wantErr bool
	}{
		{name: "valid postgres", db: databaseResource{Name: "main", Engine: store.EnginePostgres, Version: "16"}, wantErr: false},
		{name: "valid redis", db: databaseResource{Name: "cache", Engine: store.EngineRedis, Version: "7"}, wantErr: false},
		{name: "valid mysql", db: databaseResource{Name: "orders", Engine: store.EngineMySQL, Version: "8"}, wantErr: false},
		{name: "valid mongodb", db: databaseResource{Name: "docs", Engine: store.EngineMongoDB, Version: "7"}, wantErr: false},
		{name: "valid dragonfly", db: databaseResource{Name: "hotcache", Engine: store.EngineDragonfly, Version: "v1.27.1"}, wantErr: false},
		{name: "valid clickhouse", db: databaseResource{Name: "analytics", Engine: store.EngineClickHouse, Version: "24.8"}, wantErr: false},
		{name: "missing name", db: databaseResource{Engine: store.EnginePostgres, Version: "16"}, wantErr: true},
		{name: "missing engine", db: databaseResource{Name: "main", Version: "16"}, wantErr: true},
		{name: "unknown engine", db: databaseResource{Name: "main", Engine: "cassandra", Version: "7"}, wantErr: true},
		{name: "missing version", db: databaseResource{Name: "main", Engine: store.EnginePostgres}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatabaseResource(tt.db)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDatabaseResource(%+v) error = %v, wantErr %v", tt.db, err, tt.wantErr)
			}
		})
	}
}

func TestDatabaseRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/databases"},
		{http.MethodPost, "/api/v1/databases"},
		{http.MethodGet, "/api/v1/databases/main"},
		{http.MethodDelete, "/api/v1/databases/main"},
		{http.MethodGet, "/api/v1/databases/main/status"},
		{http.MethodPut, "/api/v1/databases/main/node"},
		{http.MethodPut, "/api/v1/databases/main/resources"},
		{http.MethodGet, "/api/v1/databases/main/metrics"},
		{http.MethodGet, "/api/v1/databases/main/logs"},
		{http.MethodGet, "/api/v1/databases/main/logs/stream"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleListDatabases(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var empty []databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty list, got %d", len(empty))
	}

	body := `{"name":"main","engine":"redis","version":"7"}`
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases", ""))
	var list []databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list) != 1 || list[0].Name != "main" {
		t.Fatalf("list = %+v, want one database named main", list)
	}
}

func TestHandleCreateDatabase_DuplicateName(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"main","engine":"redis","version":"7"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleCreateDatabase_InvalidBody(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"","engine":"redis","version":"7"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleGetDatabase_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetDatabase_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"main","engine":"postgres","version":"16"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Name != "main" || got.Engine != store.EnginePostgres || got.Version != "16" {
		t.Errorf("got = %+v, want main/postgres/16", got)
	}
}

func TestHandleDeleteDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"main","engine":"redis","version":"7"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/main", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteDatabase_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDatabaseStatus_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/ghost/status", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDatabaseNode_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/node", `{"node_id":"node_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.NodeID != "node_1" {
		t.Errorf("NodeID = %q, want node_1", got.NodeID)
	}

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.NodeID != "node_1" {
		t.Errorf("stored NodeID = %q, want node_1", d.NodeID)
	}
}

func TestHandleSetDatabaseNode_EmptyMovesToLocal(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node_1"); err != nil {
		t.Fatalf("seed placement: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/node", `{"node_id":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.NodeID != "" {
		t.Errorf("stored NodeID = %q, want empty (moved back to local)", d.NodeID)
	}
}

func TestHandleSetDatabaseNode_UnknownNode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/node", `{"node_id":"ghost"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleSetDatabaseNode_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/ghost/node", `{"node_id":""}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetDatabaseResources_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	body := `{"resources":{"memory_bytes":536870912,"nano_cpus":500000000}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/resources", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Resources == nil || got.Resources.MemoryBytes != 536870912 || got.Resources.NanoCPUs != 500000000 {
		t.Errorf("Resources = %+v, want {MemoryBytes:536870912 NanoCPUs:500000000}", got.Resources)
	}

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.Resources == nil || d.Resources.MemoryBytes != 536870912 || d.Resources.NanoCPUs != 500000000 {
		t.Errorf("stored Resources = %+v, want {MemoryBytes:536870912 NanoCPUs:500000000}", d.Resources)
	}
	// Engine/Version must survive the round trip unchanged: this endpoint
	// only ever touches Resources, never the rest of the record.
	if d.Engine != store.EngineRedis || d.Version != "7" {
		t.Errorf("Engine/Version = %q/%q, want unchanged redis/7", d.Engine, d.Version)
	}
}

// TestHandleSetDatabaseResources_NilClears confirms a request with no
// resources field (or an explicit null) clears a previously-set limit
// back to nil, the same full-replace semantics setDatabaseNode/
// setDatabaseProject already establish for their own fields.
func TestHandleSetDatabaseResources_NilClears(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: "main", Engine: store.EngineRedis, Version: "7",
		Resources: &store.ServiceResources{MemoryBytes: 1024, NanoCPUs: 1000},
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/resources", `{"resources":null}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.Resources != nil {
		t.Errorf("stored Resources = %+v, want nil", d.Resources)
	}
}

func TestHandleSetDatabaseResources_DatabaseNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/ghost/resources", `{"resources":null}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleSetDatabaseResources_ReadOnlyToken_Forbidden proves PUT
// /databases/{name}/resources really sits behind AbilityWrite, not just
// any authenticated caller: a token scoped only to AbilityRead must be
// rejected with 403, the same real-request proof
// TestHandleUpdateIngressSettings_PlainWriteToken_Forbidden establishes
// for its own route rather than relying on router registration alone.
func TestHandleSetDatabaseResources_ReadOnlyToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	const plaintext = "read-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_read", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"resources":{"memory_bytes":536870912,"nano_cpus":500000000}}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/databases/main/resources", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a read-only token must not reach the resources route", rec.Code, http.StatusForbidden)
	}

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.Resources != nil {
		t.Errorf("stored Resources = %+v, want nil: a rejected request must not touch the row", d.Resources)
	}
}

func TestHandleDatabaseStatus_NoConditionsYetIsEmptyList(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"main","engine":"redis","version":"7"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var conditions []any
	if err := json.Unmarshal(rec.Body.Bytes(), &conditions); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(conditions) != 0 {
		t.Errorf("conditions = %+v, want empty (no reconcile has run against this test router)", conditions)
	}
}
