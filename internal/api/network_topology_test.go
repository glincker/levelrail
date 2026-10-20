package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetNetworkTopology_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/topology", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkTopologyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Zone != "mesh.internal" {
		t.Errorf("Zone = %q, want %q", got.Zone, "mesh.internal")
	}
	if len(got.Nodes) != 0 || len(got.Apps) != 0 || len(got.Databases) != 0 || len(got.Connections) != 0 {
		t.Errorf("got = %+v, want every list empty", got)
	}
}

// TestHandleGetNetworkTopology_Assembled proves the handler is a straight
// join, not a re-derivation: a node's mesh address, a service's placement,
// a database's placement, and both DatabaseEnv- and DatabaseAttachment-
// sourced connections all show up exactly as stored.
func TestHandleGetNetworkTopology_Assembled(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithLoadBalancers(db, loadbalancer.NewRegistry(), nil))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	now := time.Now()
	if err := db.SaveNode(ctx, store.Node{ID: "node_a", Name: "alpha", Address: "10.0.0.1:9443", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save node: %v", err)
	}
	if err := db.UpdateNodeRegion(ctx, "node_a", "hetzner-fsn1"); err != nil {
		t.Fatalf("set region: %v", err)
	}
	if err := db.UpdateNodeMesh(ctx, "node_a", "pubkey-a", "100.64.0.2"); err != nil {
		t.Fatalf("set mesh: %v", err)
	}

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("save database: %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node_a"); err != nil {
		t.Fatalf("place database: %v", err)
	}

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "web:1", Port: 3000, Domains: []string{"web.example.com"},
		DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main", Field: "url"}},
	}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_a"); err != nil {
		t.Fatalf("place service: %v", err)
	}

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "worker", Image: "worker:1", Port: 4000}); err != nil {
		t.Fatalf("save second service: %v", err)
	}
	if err := db.UpdateServiceDatabaseAttachment(ctx, "worker", &store.DatabaseAttachment{DatabaseName: "main", EnvVar: "DB_URL", Field: "url"}); err != nil {
		t.Fatalf("attach database: %v", err)
	}

	if err := db.SetServiceLoadBalancer(ctx, "web", `{"algorithm":"least_conn"}`); err != nil {
		t.Fatalf("set load balancer: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/topology", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkTopologyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(got.Nodes))
	}
	node := got.Nodes[0]
	if node.ID != "node_a" || node.Region != "hetzner-fsn1" || node.MeshAddress != "100.64.0.2" {
		t.Errorf("node = %+v, want region hetzner-fsn1, mesh address 100.64.0.2", node)
	}

	if len(got.Apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(got.Apps))
	}
	byName := map[string]networkTopologyAppResource{}
	for _, a := range got.Apps {
		byName[a.Name] = a
	}
	web, ok := byName["web"]
	if !ok {
		t.Fatalf("apps = %+v, want a \"web\" entry", got.Apps)
	}
	if web.NodeID != "node_a" || web.DNSName != "web.mesh.internal" || web.MeshAddress != "100.64.0.2" {
		t.Errorf("web app = %+v, want node_a / web.mesh.internal / 100.64.0.2", web)
	}
	if len(web.Domains) != 1 || web.Domains[0] != "web.example.com" {
		t.Errorf("web app domains = %+v, want [web.example.com]", web.Domains)
	}

	if len(got.Databases) != 1 {
		t.Fatalf("got %d databases, want 1", len(got.Databases))
	}
	if gotDB := got.Databases[0]; gotDB.Name != "main" || gotDB.Engine != "postgres" || gotDB.DNSName != "main.mesh.internal" || gotDB.MeshAddress != "100.64.0.2" {
		t.Errorf("database = %+v, want main / postgres / main.mesh.internal / 100.64.0.2", gotDB)
	}

	if len(got.Connections) != 2 {
		t.Fatalf("got %d connections, want 2 (one from DatabaseEnv, one from DatabaseAttachment): %+v", len(got.Connections), got.Connections)
	}
	for _, c := range got.Connections {
		if c.Database != "main" {
			t.Errorf("connection %+v, want database main", c)
		}
	}

	if len(got.LoadBalancers) != 1 || got.LoadBalancers[0].Service != "web" || got.LoadBalancers[0].Algorithm != "least_conn" {
		t.Errorf("load balancers = %+v, want one entry for web/least_conn", got.LoadBalancers)
	}
}

func TestHandleGetNetworkTopology_UnplacedServiceUsesLocalNode(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	now := time.Now()
	if err := db.SaveNode(ctx, store.Node{ID: "node_local", Name: "local", Status: store.NodeStatusOnline, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save node: %v", err)
	}
	if err := db.UpdateNodeMesh(ctx, "node_local", "pubkey-local", "100.64.0.1"); err != nil {
		t.Fatalf("set mesh: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "api", Image: "api:1", Port: 8080}); err != nil {
		t.Fatalf("save service: %v", err)
	}
	rt.SetLocalNodeID("node_local")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/network/topology", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got networkTopologyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Apps) != 1 {
		t.Fatalf("got %d apps, want 1", len(got.Apps))
	}
	// NodeID stays "" on the wire (the raw placement value), but the mesh
	// address still resolves through the local-node sentinel, matching
	// internal/network.BuildRecords' own resolution.
	if got.Apps[0].NodeID != "" || got.Apps[0].MeshAddress != "100.64.0.1" {
		t.Errorf("app = %+v, want empty node_id and the local node's mesh address", got.Apps[0])
	}
}

func TestHandleGetNetworkTopology_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/topology", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleSetNodeRegion(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/nodes/node_a/region", `{"region":"aws-us-east-1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got nodeResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Region != "aws-us-east-1" {
		t.Errorf("Region = %q, want %q", got.Region, "aws-us-east-1")
	}
}

func TestHandleSetNodeRegion_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/nodes/missing/region", `{"region":"home-lab"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
