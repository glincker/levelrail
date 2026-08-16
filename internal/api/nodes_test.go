package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/nodehealth"
	"github.com/GLINCKER/levelrail/internal/store"
)

func seedNode(t *testing.T, db *store.DB, id, name string) {
	t.Helper()
	now := time.Now()
	if err := db.SaveNode(context.Background(), store.Node{
		ID: id, Name: name, Address: "10.0.0.1:9443", Status: store.NodeStatusPending,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed node %q: %v", name, err)
	}
}

func TestHandleListNodes_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d nodes, want 0", len(got))
	}
}

func TestHandleListNodes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_b", "zebra")
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zebra" {
		t.Fatalf("got = %+v, want [alpha zebra] in that order", got)
	}
	if got[0].Status != string(store.NodeStatusPending) {
		t.Errorf("got[0].Status = %q, want %q", got[0].Status, store.NodeStatusPending)
	}
}

func TestHandleGetNode(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "node_a" || got.Name != "alpha" {
		t.Errorf("got = %+v, want ID=node_a Name=alpha", got)
	}
}

func TestHandleGetNode_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/nonexistent", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteNode_Idempotent(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/node_a", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first delete status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/node_a", ""))
	if rec.Code != http.StatusNoContent {
		t.Errorf("second (already-gone) delete status = %d, want %d (idempotent)", rec.Code, http.StatusNoContent)
	}

	// Confirmed gone, not just a 204 the handler returned unconditionally.
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GetNode after delete status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleSetNodeWorkloads_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/nodes/node_a/workloads",
		`{"accepts_app_workloads":false,"accepts_build_workloads":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.AcceptsAppWorkloads != false || got.AcceptsBuildWorkloads != true {
		t.Errorf("got = %+v, want AcceptsAppWorkloads=false AcceptsBuildWorkloads=true", got)
	}

	stored, err := db.GetNode(context.Background(), "node_a")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if stored.AcceptsAppWorkloads != false || stored.AcceptsBuildWorkloads != true {
		t.Errorf("stored = %+v, want AcceptsAppWorkloads=false AcceptsBuildWorkloads=true", stored)
	}
}

func TestHandleSetNodeWorkloads_UnknownNode_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/nodes/nonexistent/workloads",
		`{"accepts_app_workloads":true,"accepts_build_workloads":true}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSetNodeWorkloads_InvalidBody_BadRequest(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/nodes/node_a/workloads", `not json`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateNodeJoinToken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got createNodeJoinTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Token == "" {
		t.Error("Token is empty, want a generated plaintext token")
	}
	if !got.ExpiresAt.After(time.Now()) {
		t.Errorf("ExpiresAt = %v, want a time in the future", got.ExpiresAt)
	}

	// The plaintext must actually have landed in the store, hashed, not
	// just returned in the response and discarded.
	stored, err := db.GetNodeJoinTokenByHash(context.Background(), hashToken(got.Token))
	if err != nil {
		t.Fatalf("GetNodeJoinTokenByHash() error = %v", err)
	}
	if stored.UsedAt != nil {
		t.Error("stored.UsedAt is set, want nil for a freshly minted token")
	}
}

func TestHandleCreateNodeJoinToken_EachCallMintsADistinctToken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	var tokens []string
	for range 2 {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", ""))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		var got createNodeJoinTokenResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		tokens = append(tokens, got.Token)
	}
	if tokens[0] == tokens[1] {
		t.Error("two mint calls returned the same plaintext token, want each call to mint a distinct one")
	}
}

func TestNodeRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	tests := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/nodes"},
		{http.MethodGet, "/api/v1/nodes/some-id"},
		{http.MethodDelete, "/api/v1/nodes/some-id"},
		{http.MethodPut, "/api/v1/nodes/some-id/workloads"},
		{http.MethodPost, "/api/v1/nodes/join-tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestNodeRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: node routes are fleet-level, a plain write token must not reach them", rec.Code, http.StatusForbidden)
	}
}

func TestHandleCordonAndUncordonNode(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_1/cordon", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("cordon status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Schedulable {
		t.Error("Schedulable = true after cordon, want false")
	}
	if got.Status != string(store.NodeStatusPending) {
		t.Errorf("Status = %q after cordon, want unchanged %q (cordon must not touch Status)", got.Status, store.NodeStatusPending)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_1/uncordon", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("uncordon status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Schedulable {
		t.Error("Schedulable = false after uncordon, want true")
	}
}

func TestHandleCordonNode_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/nonexistent/cordon", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetNodeHealth_EmptyBeforeAnyReconcile(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_1", "worker-1")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_1/health", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []reconcile.Condition
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d conditions, want 0 before any reconcile has run", len(got))
	}
}

func TestHandleGetNodeHealth_SurfacesStoredCondition(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")

	if err := db.UpsertConditions(ctx, nodeHealthControllerName("node_1"), []reconcile.Condition{{
		Type: nodehealth.ConditionType, Status: reconcile.ConditionFalse, Reason: "HeartbeatStale", Message: "no heartbeat for 90s",
	}}); err != nil {
		t.Fatalf("seed condition: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_1/health", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []reconcile.Condition
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Reason != "HeartbeatStale" || got[0].Status != reconcile.ConditionFalse {
		t.Fatalf("got = %+v, want one False/HeartbeatStale condition", got)
	}
}

func TestHandleGetNodeHealth_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/nonexistent/health", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDrainNode_MovesServicesAndDatabases(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatalf("place service: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node_1"); err != nil {
		t.Fatalf("place database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_1/drain", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got drainNodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TargetNodeID != "" {
		t.Errorf("TargetNodeID = %q, want empty (default local node)", got.TargetNodeID)
	}
	if len(got.MovedServices) != 1 || got.MovedServices[0] != "web" {
		t.Errorf("MovedServices = %v, want [web]", got.MovedServices)
	}
	if len(got.MovedDatabases) != 1 || got.MovedDatabases[0] != "main" {
		t.Errorf("MovedDatabases = %v, want [main]", got.MovedDatabases)
	}
	if len(got.Errors) != 0 {
		t.Errorf("Errors = %v, want none", got.Errors)
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "" {
		t.Errorf("service NodeID = %q after drain, want empty (moved to local)", svc.NodeID)
	}
	dbase, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if dbase.NodeID != "" {
		t.Errorf("database NodeID = %q after drain, want empty (moved to local)", dbase.NodeID)
	}

	// A node with nothing left placed on it is now safe to delete for
	// real: the exact "drain unblocks delete" contract handleDeleteNode's
	// own doc comment promises.
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/node_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Errorf("delete-after-drain status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestHandleDrainNode_ToExplicitTarget(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")
	seedNode(t, db, "node_2", "worker-2")

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatalf("place service: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_1/drain?target_node_id=node_2", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.NodeID != "node_2" {
		t.Errorf("service NodeID = %q after drain, want node_2", svc.NodeID)
	}
}

func TestHandleDrainNode_CordonedTarget_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")
	seedNode(t, db, "node_2", "worker-2")
	if err := db.SetNodeSchedulable(ctx, "node_2", false); err != nil {
		t.Fatalf("cordon node_2: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/node_1/drain?target_node_id=node_2", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleDrainNode_UnknownNode_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/nodes/nonexistent/drain", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// fakeDrainAppStore and fakeDrainDatabaseStore/fakeDrainNodeStore below
// are hand-written fakes (the same "not a mocking framework" convention
// internal/reconcile/database's own fakeStore already establishes),
// used only by TestHandleDrainNode_PartialFailure: every other test in
// this file exercises handleDrainNode through the real store, but a
// real *store.DB has no way to make UpdateServiceNode fail for
// specifically the second of three services on demand, which is exactly
// the case TASKS.md 3.7 calls out testing ("moving service 2 of 3 off a
// node fails partway through"). A fake store, controlled per-call, is
// the direct way to force that.
type fakeDrainAppStore struct {
	services     []store.DesiredService
	failOnUpdate map[string]error
	updateCalls  []string
}

func (f *fakeDrainAppStore) SaveDesiredService(context.Context, store.DesiredService) error {
	return nil
}
func (f *fakeDrainAppStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return nil, store.ErrServiceNotFound
}
func (f *fakeDrainAppStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return nil, nil
}
func (f *fakeDrainAppStore) DeleteDesiredService(context.Context, string) error { return nil }
func (f *fakeDrainAppStore) UpdateServiceNode(_ context.Context, name, _ string) error {
	f.updateCalls = append(f.updateCalls, name)
	if err, ok := f.failOnUpdate[name]; ok {
		return err
	}
	return nil
}
func (f *fakeDrainAppStore) ListDesiredServicesByNode(context.Context, string) ([]store.DesiredService, error) {
	return f.services, nil
}
func (f *fakeDrainAppStore) RestartService(context.Context, string) error { return nil }
func (f *fakeDrainAppStore) UpdateServiceProject(context.Context, string, string) error {
	return nil
}
func (f *fakeDrainAppStore) UpdateServiceStorageTarget(context.Context, string, string) error {
	return nil
}
func (f *fakeDrainAppStore) UpdateServiceSuspended(context.Context, string, bool) error {
	return nil
}

type fakeDrainDatabaseStore struct{}

func (fakeDrainDatabaseStore) SaveDesiredDatabase(context.Context, store.DesiredDatabase) error {
	return nil
}
func (fakeDrainDatabaseStore) GetDesiredDatabase(context.Context, string) (*store.DesiredDatabase, error) {
	return nil, store.ErrDatabaseNotFound
}
func (fakeDrainDatabaseStore) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return nil, nil
}
func (fakeDrainDatabaseStore) DeleteDesiredDatabase(context.Context, string) error { return nil }
func (fakeDrainDatabaseStore) UpdateDatabaseNode(context.Context, string, string) error {
	return nil
}
func (fakeDrainDatabaseStore) ListDesiredDatabasesByNode(context.Context, string) ([]store.DesiredDatabase, error) {
	return nil, nil
}
func (fakeDrainDatabaseStore) UpdateDatabaseProject(context.Context, string, string) error {
	return nil
}
func (fakeDrainDatabaseStore) SetDatabaseBackupSchedule(context.Context, string, string, string, int) error {
	return nil
}
func (fakeDrainDatabaseStore) SetDatabasePublicAccess(context.Context, string, bool, int) (int, error) {
	return 0, nil
}

type fakeDrainNodeStore struct {
	node *store.Node
}

func (f *fakeDrainNodeStore) ListNodes(context.Context) ([]store.Node, error) { return nil, nil }
func (f *fakeDrainNodeStore) GetNode(_ context.Context, id string) (*store.Node, error) {
	if f.node != nil && f.node.ID == id {
		cp := *f.node
		return &cp, nil
	}
	return nil, store.ErrNodeNotFound
}
func (f *fakeDrainNodeStore) DeleteNode(context.Context, string) error { return nil }
func (f *fakeDrainNodeStore) SaveNodeJoinToken(context.Context, store.NodeJoinToken) error {
	return nil
}
func (f *fakeDrainNodeStore) SetNodeSchedulable(context.Context, string, bool) error { return nil }
func (f *fakeDrainNodeStore) UpdateNodeWorkloads(context.Context, string, bool, bool) error {
	return nil
}

// TestHandleDrainNode_PartialFailure is the exact scenario TASKS.md 3.7
// calls out: moving service 2 of 3 off a node fails partway through.
// Reconcile must (a) surface the failure rather than silently reporting
// full success, (b) keep going and still attempt every remaining
// service rather than stopping at the first error, and (c) leave every
// service that did succeed actually moved.
func TestHandleDrainNode_PartialFailure(t *testing.T) {
	apps := &fakeDrainAppStore{
		services: []store.DesiredService{
			{Name: "api", NodeID: "node_1"},
			{Name: "web", NodeID: "node_1"},
			{Name: "worker", NodeID: "node_1"},
		},
		failOnUpdate: map[string]error{"web": errors.New("write conflict")},
	}
	nodes := &fakeDrainNodeStore{node: &store.Node{ID: "node_1", Name: "worker-1", Schedulable: true}}

	rt := &Router{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		apps:      apps,
		databases: fakeDrainDatabaseStore{},
		nodes:     nodes,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node_1/drain", nil)
	req.SetPathValue("id", "node_1")
	rec := httptest.NewRecorder()
	rt.handleDrainNode(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}

	var got drainNodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.MovedServices) != 2 || got.MovedServices[0] != "api" || got.MovedServices[1] != "worker" {
		t.Fatalf("MovedServices = %v, want [api worker] (drain must continue past the one failure)", got.MovedServices)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly 1 entry for the failed service", got.Errors)
	}
	if len(apps.updateCalls) != 3 {
		t.Fatalf("UpdateServiceNode call count = %d, want 3: every listed service must be attempted, not just the ones before the failure", len(apps.updateCalls))
	}
}

func TestHandleDeleteNode_BlockedByPlacement(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedNode(t, db, "node_1", "worker-1")

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_1"); err != nil {
		t.Fatalf("place service: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/nodes/node_1", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}

	// Refused delete must not have removed the node.
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_1", ""))
	if rec.Code != http.StatusOK {
		t.Errorf("GetNode after refused delete status = %d, want %d (node must still exist)", rec.Code, http.StatusOK)
	}
}

func TestNodeRoutes_RootToken_Succeeds(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	const plaintext = "root-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_root", Name: "root", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
