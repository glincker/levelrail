package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
