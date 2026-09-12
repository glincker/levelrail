package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// mustCreateDatabase POSTs body to /api/v1/databases and requires a 201.
func mustCreateDatabase(t *testing.T, rt *Router, cookie *http.Cookie, body string) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

// getDatabaseResource GETs /api/v1/databases/{name} and decodes the body.
func getDatabaseResource(t *testing.T, rt *Router, cookie *http.Cookie, name string) databaseResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/"+name, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

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

	assertRoutesRequireAuth(t, rt, []routeCase{
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
	})
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

	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7"}`)

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

// TestHandleListDatabases_Status covers the batched status field
// (databaseListResource), the same one-healthy/one-broken/one-pending
// shape TestHandleListApps_Status covers for apps.
func TestHandleListDatabases_Status(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	for _, name := range []string{"healthy-db", "broken-db", "pending-db"} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
			Name: name, Engine: store.EnginePostgres, Version: "16",
		}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	seedTriStateConditions(t, db, "database", "healthy-db", "broken-db")
	// pending-db deliberately gets no UpsertConditions call at all.

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []databaseListResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d databases, want 3", len(got))
	}

	byName := map[string]databaseListResource{}
	for _, d := range got {
		byName[d.Name] = d
	}

	if s := byName["healthy-db"].Status; s.Label != "Healthy" || s.Variant != "success" {
		t.Errorf("healthy-db status = %+v, want Healthy/success", s)
	}
	if s := byName["broken-db"].Status; s.Label != "Attention needed" || s.Variant != "destructive" {
		t.Errorf("broken-db status = %+v, want Attention needed/destructive", s)
	}
	if s := byName["pending-db"].Status; s.Label != "No status yet" || s.Variant != "muted" {
		t.Errorf("pending-db status = %+v, want No status yet/muted", s)
	}
}

func TestHandleCreateDatabase_DuplicateName(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"main","engine":"redis","version":"7"}`
	mustCreateDatabase(t, rt, cookie, body)

	rec := httptest.NewRecorder()
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

// TestHandleCreateDatabase_UnreadableBody_Returns400 mirrors
// TestHandleCreateApp_UnreadableBody_Returns400: handleCreateDatabase
// also reads the raw body first to probe for an explicit node_id key
// (nodeIDKeyPresent) before decoding it.
func TestHandleCreateDatabase_UnreadableBody_Returns400(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	assertUnreadableBodyRejected(t, rt, cookie, "/api/v1/databases")
}

// TestHandleCreateDatabase_MalformedJSON_Returns400 covers the
// json.Unmarshal error path, distinct from TestHandleCreateDatabase_InvalidBody's
// semantically-invalid-but-well-formed body.
func TestHandleCreateDatabase_MalformedJSON_Returns400(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleCreateDatabase_CordonedNode_Rejected mirrors
// TestHandleCreateApp_CordonedNode_Rejected for the database create path.
func TestHandleCreateDatabase_CordonedNode_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")
	if err := db.SetNodeSchedulable(ctx, "node_1", false); err != nil {
		t.Fatalf("cordon node: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7","node_id":"node_1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(ctx, "main"); err == nil {
		t.Error("a cordoned node_id must not have saved the database")
	}
}

// TestHandleCreateDatabase_ValidateNodeGenericError_Returns500 mirrors
// TestHandleCreateApp_ValidateNodeGenericError_Returns500 for the
// database create path.
func TestHandleCreateDatabase_ValidateNodeGenericError_Returns500(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.nodes = &erroringNodeStore{NodeStore: rt.nodes, getNodeErr: errors.New("node lookup exploded")}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7","node_id":"node_1"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "main"); err == nil {
		t.Error("a validate-node failure must not have saved the database")
	}
}

// TestHandleCreateDatabase_AutoPlaceNodeError_Returns500 mirrors
// TestHandleCreateApp_AutoPlaceNodeError_Returns500 for the database
// create path.
func TestHandleCreateDatabase_AutoPlaceNodeError_Returns500(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.nodes = &erroringNodeStore{NodeStore: rt.nodes, listNodesErr: errors.New("list nodes exploded")}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7"}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "main"); err == nil {
		t.Error("an auto-place failure must not have saved the database")
	}
}

// TestHandleCreateDatabase_AutoPlacement mirrors
// TestHandleCreateApp_AutoPlacement (apps_test.go): node_id omitted picks
// the least-loaded registered node, an explicit node_id (including an
// explicit "") is always honored as an override, and a single-node
// install keeps today's local-node behavior unchanged.
func TestHandleCreateDatabase_AutoPlacement(t *testing.T) {
	ctx := context.Background()

	t.Run("no other nodes registered: stays local, not auto-placed", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)

		got := createResourceViaAPI[databaseResource](t, rt, cookie, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7"}`, http.StatusCreated)
		assertAutoPlacementResult(t, got.NodeID, got.AutoPlaced, "", false)
	})

	t.Run("node_id omitted with multiple nodes registered: auto-placed on the least-loaded one", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		seedOnlineNode(t, db, "node_a", "alpha", true)
		seedOnlineNode(t, db, "node_b", "bravo", true)
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "existing", Image: "img:1", Port: 80}); err != nil {
			t.Fatalf("seed existing service: %v", err)
		}
		if err := db.UpdateServiceNode(ctx, "existing", "node_a"); err != nil {
			t.Fatalf("place existing service: %v", err)
		}

		got := createResourceViaAPI[databaseResource](t, rt, cookie, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7"}`, http.StatusCreated)
		assertAutoPlacementResult(t, got.NodeID, got.AutoPlaced, "node_b", true)

		saved, err := db.GetDesiredDatabase(ctx, "main")
		if err != nil {
			t.Fatalf("GetDesiredDatabase: %v", err)
		}
		if saved.NodeID != "node_b" {
			t.Errorf("persisted NodeID = %q, want %q", saved.NodeID, "node_b")
		}
	})

	t.Run("explicit node_id overrides auto-placement", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		seedOnlineNode(t, db, "node_a", "alpha", true)
		seedOnlineNode(t, db, "node_b", "bravo", true)

		got := createResourceViaAPI[databaseResource](t, rt, cookie, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7","node_id":"node_a"}`, http.StatusCreated)
		assertAutoPlacementResult(t, got.NodeID, got.AutoPlaced, "node_a", false)
	})

	t.Run("explicit empty node_id overrides auto-placement, stays local", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		seedOnlineNode(t, db, "node_a", "alpha", true)
		seedOnlineNode(t, db, "node_b", "bravo", true)

		got := createResourceViaAPI[databaseResource](t, rt, cookie, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7","node_id":""}`, http.StatusCreated)
		assertAutoPlacementResult(t, got.NodeID, got.AutoPlaced, "", false)
	})

	t.Run("explicit node_id for an unknown node is rejected", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)

		createResourceViaAPI[databaseResource](t, rt, cookie, "/api/v1/databases", `{"name":"main","engine":"redis","version":"7","node_id":"does-not-exist"}`, http.StatusBadRequest)
		if _, err := db.GetDesiredDatabase(ctx, "main"); err == nil {
			t.Error("a rejected node_id must not have saved the database")
		}
	})
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

	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres","version":"16"}`)

	got := getDatabaseResource(t, rt, cookie, "main")
	if got.Name != "main" || got.Engine != store.EnginePostgres || got.Version != "16" {
		t.Errorf("got = %+v, want main/postgres/16", got)
	}
}

// TestHandleGetDatabase_TLSEnabled proves databaseResource.TLSEnabled
// reflects whether this database's TLS certificate has actually been
// generated (database.TLSCertEnvKey), not just whether its engine
// supports TLS: a mysql database (SupportsTLS == false) never reports
// it true even with a secrets manager configured, and a postgres
// database only reports it true once the secret actually exists.
func TestHandleGetDatabase_TLSEnabled(t *testing.T) {
	setter := &fakeSecretSetter{existsValues: map[string]bool{
		"main/tls_cert": true,
	}}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)

	for _, body := range []string{
		`{"name":"main","engine":"postgres","version":"16"}`,
		`{"name":"orders","engine":"mysql","version":"8"}`,
	} {
		mustCreateDatabase(t, rt, cookie, body)
	}

	tests := []struct {
		name string
		want bool
	}{
		{name: "main", want: true},    // postgres, cert exists
		{name: "orders", want: false}, // mysql: SupportsTLS is false regardless of secrets
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDatabaseResource(t, rt, cookie, tt.name)
			if got.TLSEnabled != tt.want {
				t.Errorf("TLSEnabled = %v, want %v", got.TLSEnabled, tt.want)
			}
		})
	}
}

// TestHandleGetDatabase_TLSEnabled_NoSecretsManager proves TLSEnabled
// stays false when no secrets manager is configured at all, the same
// "no master key means no TLS" fallback WithTLS's own doc comment
// establishes at the reconciler level.
func TestHandleGetDatabase_TLSEnabled_NoSecretsManager(t *testing.T) {
	rt, db := newTestRouter(t) // no WithSecretSetter
	cookie := loginTestSession(t, rt, db)

	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres","version":"16"}`)

	got := getDatabaseResource(t, rt, cookie, "main")
	if got.TLSEnabled {
		t.Errorf("TLSEnabled = true, want false with no secrets manager configured")
	}
}

func TestHandleDeleteDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7"}`)

	rec := httptest.NewRecorder()
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

// seedDatabaseOnNode seeds "main" (redis:7, unplaced) and registers
// nodeID, the shared precondition TestHandleSetDatabaseNode_Success/
// TeardownDispatchesOnOldNode/NoExecRuntime_NoTeardown all need before
// moving "main" onto nodeID.
func seedDatabaseOnNode(t *testing.T, db *store.DB, nodeID string) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	seedNode(t, db, nodeID, "worker-1")
}

// seedDatabasePlacedOnNode is seedDatabaseOnNode plus actually placing
// "main" on nodeID, the shared precondition
// TestHandleSetDatabaseNode_TeardownResolveFailure/TeardownInspectFailure
// both need before moving "main" back to local.
func seedDatabasePlacedOnNode(t *testing.T, db *store.DB, nodeID string) {
	t.Helper()
	seedDatabaseOnNode(t, db, nodeID)
	if err := db.UpdateDatabaseNode(context.Background(), "main", nodeID); err != nil {
		t.Fatalf("seed placement: %v", err)
	}
}

// putDatabaseNodeViaAPI PUTs body to /api/v1/databases/main/node and
// requires 200 OK, returning the response for the caller's own
// assertions. Shared by TestHandleSetDatabaseNode_*'s repeated move-then-
// assert-200 tail.
func putDatabaseNodeViaAPI(t *testing.T, rt *Router, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/node", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	return rec
}

func TestHandleSetDatabaseNode_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedDatabaseOnNode(t, db, "node_1")

	rec := putDatabaseNodeViaAPI(t, rt, cookie, `{"node_id":"node_1"}`)

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

// TestHandleSetDatabaseNode_TeardownDispatchesOnOldNode proves moving a
// database to a different node tears down the container left running on
// the OLD node, the database counterpart to
// TestHandleSetAppNode_TeardownDispatchesOnOldNode.
func TestHandleSetDatabaseNode_TeardownDispatchesOnOldNode(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectByNameCalls: make(chan struct{}, 4)}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedDatabaseOnNode(t, db, "node_1")

	putDatabaseNodeViaAPI(t, rt, cookie, `{"node_id":"node_1"}`)

	select {
	case <-fake.inspectByNameCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the background teardown to inspect the old node's container")
	}
}

// TestHandleSetDatabaseNode_SameNode_NoTeardown proves setting the same
// node_id a database already has does not dispatch a teardown.
func TestHandleSetDatabaseNode_SameNode_NoTeardown(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectByNameCalls: make(chan struct{}, 4)}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/node", `{"node_id":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	select {
	case <-fake.inspectByNameCalls:
		t.Fatal("teardown dispatched for a no-op node move")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestHandleSetDatabaseNode_NoExecRuntime_NoTeardown proves moving a
// database when no exec runtime is configured is a safe no-op, mirroring
// TestHandleDeleteApp_TeardownResolveFailure_StillDeletes's own
// not-configured shape for teardownServiceContainers.
func TestHandleSetDatabaseNode_NoExecRuntime_NoTeardown(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedDatabaseOnNode(t, db, "node_1")

	putDatabaseNodeViaAPI(t, rt, cookie, `{"node_id":"node_1"}`)

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.NodeID != "node_1" {
		t.Errorf("stored NodeID = %q, want node_1", d.NodeID)
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

// TestHandleSetDatabaseNode_LoadExistingGenericError_Returns500 mirrors
// TestHandleSetAppNode_LoadExistingGenericError_Returns500 for the
// database node-move path.
func TestHandleSetDatabaseNode_LoadExistingGenericError_Returns500(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	rt.databases = &erroringDatabaseStore{DatabaseStore: rt.databases, getDesiredDatabaseErr: errors.New("load existing exploded")}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/node", `{"node_id":""}`))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

// TestHandleSetDatabaseNode_TeardownResolveFailure covers
// teardownDatabaseContainer's own resolve-failure branch: this runs
// synchronously (resolving the node's runtime happens before the
// background Teardown goroutine is even started), so no channel wait is
// needed to observe it, mirroring TestHandleDeleteApp_TeardownResolveFailure_StillDeletes's
// own always-failing resolver.
func TestHandleSetDatabaseNode_TeardownResolveFailure(t *testing.T) {
	db := openTestDB(t)
	resolver := func(string) (docker.Runtime, error) { return nil, errors.New("node offline") }
	rt := NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(resolver))
	cookie := loginTestSession(t, rt, db)
	seedDatabasePlacedOnNode(t, db, "node_1")

	putDatabaseNodeViaAPI(t, rt, cookie, `{"node_id":""}`)
}

// TestHandleSetDatabaseNode_TeardownInspectFailure covers Teardown itself
// failing inside the background goroutine (its own InspectByName call
// errors), the counterpart to TestHandleSetDatabaseNode_TeardownDispatchesOnOldNode's
// success path. Waiting on inspectByNameCalls is enough to know Teardown
// has run and returned its error, the same synchronization that test
// already establishes.
func TestHandleSetDatabaseNode_TeardownInspectFailure(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectErr: errors.New("inspect failed"), inspectByNameCalls: make(chan struct{}, 4)}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	seedDatabasePlacedOnNode(t, db, "node_1")

	putDatabaseNodeViaAPI(t, rt, cookie, `{"node_id":""}`)

	select {
	case <-fake.inspectByNameCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the background teardown to inspect the old node's container")
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

// TestHandleSetDatabaseResources_AppliedLive mirrors
// TestHandleUpdateApp_ResourcesAppliedLive for databases: a running
// container gets the new limits pushed immediately via UpdateResources,
// reported back as resources_applied_live.
func TestHandleSetDatabaseResources_AppliedLive(t *testing.T) {
	fake := &fakeExecAppRuntime{inspectState: &docker.ContainerState{ID: "db-c1", Running: true}}
	rt, db := newTestRouterWithExecRuntime(t, fake)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	body := `{"resources":{"memory_bytes":536870912}}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/resources", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.ResourcesAppliedLive {
		t.Error("resources_applied_live = false, want true: a running container should get the update immediately")
	}
	if fake.updateResourcesCalls != 1 || fake.updateResourcesID != "db-c1" {
		t.Errorf("updateResourcesCalls=%d updateResourcesID=%q, want 1/db-c1", fake.updateResourcesCalls, fake.updateResourcesID)
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

	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"redis","version":"7"}`)

	rec := httptest.NewRecorder()
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
