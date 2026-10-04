package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetNetworkProxy_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/proxy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkProxyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Domains) != 0 {
		t.Errorf("Domains = %+v, want none", got.Domains)
	}
}

func TestHandleGetNetworkProxy_LocalPlacementIsReachable(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveServiceOnNode(t, db, "web", "", []string{"web.example.com"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/proxy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkProxyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Domains) != 1 {
		t.Fatalf("got %d domains, want 1: %+v", len(got.Domains), got.Domains)
	}
	row := got.Domains[0]
	if row.Domain != "web.example.com" || row.App != "web" {
		t.Errorf("row = %+v, want web.example.com/web", row)
	}
	if !row.Reachable {
		t.Errorf("Reachable = false, want true: placed locally")
	}
	if row.FixCommand != "" {
		t.Errorf("FixCommand = %q, want empty: reachable rows need no fix", row.FixCommand)
	}
}

// TestHandleGetNetworkProxy_RemotePlacementIsUnreachable proves this
// endpoint surfaces the exact gap doctor_cross_node_ingress.go's check
// flags: an app with domains, placed on a different node, has no mesh
// path to it yet.
func TestHandleGetNetworkProxy_RemotePlacementIsUnreachable(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveNode(ctx, store.Node{ID: "node-2", Name: "worker-1", Address: "161.35.113.211", Status: store.NodeStatusOnline, Schedulable: true}); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	saveServiceOnNode(t, db, "static-test", "node-2", []string{"static.example.com"})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/proxy", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkProxyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Domains) != 1 {
		t.Fatalf("got %d domains, want 1: %+v", len(got.Domains), got.Domains)
	}
	row := got.Domains[0]
	if row.Reachable {
		t.Errorf("Reachable = true, want false: static-test is placed on node-2, not local")
	}
	if row.NodeName != "worker-1" {
		t.Errorf("NodeName = %q, want worker-1", row.NodeName)
	}
	if row.FixCommand == "" {
		t.Error("FixCommand = \"\", want a concrete next step")
	}
}

func TestHandleGetNetworkProxy_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/proxy", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
