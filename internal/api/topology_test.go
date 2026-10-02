package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestTopologyRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/projects/proj_1/topology"},
	})
}

func TestHandleGetProjectTopology_UnknownProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/does-not-exist/topology", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleGetProjectTopology(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "shop", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.UpdateDatabaseProject(ctx, "main", "proj_1"); err != nil {
		t.Fatalf("UpdateDatabaseProject() error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name:  "web",
		Image: "web:latest",
		DatabaseEnv: map[string]store.DatabaseEnvRef{
			"DATABASE_URL": {Database: "main", Field: "url"},
		},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_1/topology", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got topologyGraph
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("Nodes = %+v, want 2 (one app, one database)", got.Nodes)
	}
	if len(got.Edges) != 1 || got.Edges[0].Kind != TopologyEdgeDatabaseBinding {
		t.Fatalf("Edges = %+v, want one database_binding edge", got.Edges)
	}
}
