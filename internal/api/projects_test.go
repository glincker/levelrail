package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestProjectsRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/projects"},
		{http.MethodPost, "/api/v1/projects"},
		{http.MethodGet, "/api/v1/projects/proj_1"},
		{http.MethodDelete, "/api/v1/projects/proj_1"},
		{http.MethodPut, "/api/v1/apps/web/project"},
		{http.MethodPut, "/api/v1/databases/main/project"},
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

func TestHandleCreateProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", `{"name":"my-saas"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got projectResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "my-saas" || got.ID == "" || got.CreatedAt == "" {
		t.Errorf("response = %+v, want a real id, name my-saas, non-empty created_at", got)
	}

	stored, err := db.GetProject(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if stored.Name != "my-saas" {
		t.Errorf("stored project name = %q, want my-saas", stored.Name)
	}
}

func TestHandleCreateProject_MissingName_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCreateProject_MalformedBody_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", `{not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleCreateProject_DuplicateNamesAllowed documents, at the API
// layer, the same deliberate absence of a uniqueness check
// store.TestListProjects_DuplicateNamesAllowed documents at the store
// layer: a project is addressed by id, never by name.
func TestHandleCreateProject_DuplicateNamesAllowed(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"shared-name"}`
	rec1 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec1, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", body))
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d", rec1.Code, http.StatusCreated)
	}
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects", body))
	if rec2.Code != http.StatusCreated {
		t.Fatalf("second create status = %d, want %d (duplicate names are allowed)", rec2.Code, http.StatusCreated)
	}
}

func TestHandleListProjects(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_a", Name: "a", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_b", Name: "b", CreatedAt: "2026-08-14T00:00:01Z"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got []projectResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestHandleGetProject_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleDeleteProject_ResourcesBecomeProjectLess is the API-layer
// counterpart to store.TestDeleteProject_ServiceBecomesProjectLess: the
// documented, deliberate behavior (not a gap) that deleting a project
// leaves its apps/databases running, simply project-less again.
func TestHandleDeleteProject_ResourcesBecomeProjectLess(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("seed project assignment: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/projects/proj_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() after project delete error = %v, want the app to still exist", err)
	}
	if svc.ProjectID != "" {
		t.Errorf("ProjectID = %q after owning project deleted, want empty", svc.ProjectID)
	}
}

func TestHandleDeleteProject_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/projects/ghost", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleCreateApp_WithProjectID covers handleCreateApp's own
// deliberate exception (see that handler's doc comment): unlike
// PUT-driven updates, a create request may assign a project directly.
func TestHandleCreateApp_WithProjectID(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	body := `{"name":"web","image":"levelrail/web:1","port":3000,"project_id":"proj_1"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("response ProjectID = %q, want proj_1", got.ProjectID)
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.ProjectID != "proj_1" {
		t.Errorf("stored ProjectID = %q, want proj_1", svc.ProjectID)
	}
}

func TestHandleCreateApp_UnknownProjectID_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"name":"web","image":"levelrail/web:1","port":3000,"project_id":"ghost"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err == nil {
		t.Error("a create rejected for an unknown project_id must not have saved the app")
	}
}

// TestHandleUpdateApp_DoesNotChangeProject is the update-side regression
// this feature's whole "narrow, dedicated mutation" design exists to
// prevent: an ordinary PUT (e.g. editing image/port) must never silently
// move an app between projects, mirroring the existing NodeID guarantee.
func TestHandleUpdateApp_DoesNotChangeProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("seed project assignment: %v", err)
	}

	// An ordinary update, with no project_id in the body at all (and
	// even trying a *different* project_id would still be ignored, this
	// endpoint reads nothing from req.ProjectID).
	update := `{"name":"web","image":"levelrail/web:2","port":3000,"project_id":"proj_should_be_ignored"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", update))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("response ProjectID = %q, want proj_1 (unchanged)", got.ProjectID)
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.Image != "levelrail/web:2" {
		t.Errorf("Image = %q, want levelrail/web:2 (the update itself must still take effect)", svc.Image)
	}
	if svc.ProjectID != "proj_1" {
		t.Errorf("stored ProjectID = %q, want proj_1 (an ordinary update must not silently change project)", svc.ProjectID)
	}
}

func TestHandleSetAppProject_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/project", `{"project_id":"proj_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1", got.ProjectID)
	}
}

func TestHandleSetAppProject_EmptyMovesToNoProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/project", `{"project_id":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.ProjectID != "" {
		t.Errorf("stored ProjectID = %q, want empty (moved back to no project)", svc.ProjectID)
	}
}

func TestHandleSetAppProject_UnknownProject_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/project", `{"project_id":"ghost"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if svc.ProjectID != "" {
		t.Errorf("stored ProjectID = %q, want unchanged (empty): a rejected request must not partially apply", svc.ProjectID)
	}
}

func TestHandleSetAppProject_UnknownApp_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/ghost/project", `{"project_id":""}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCreateDatabase_WithProjectID(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	body := `{"name":"main","engine":"postgres","version":"16","project_id":"proj_1"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	d, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if d.ProjectID != "proj_1" {
		t.Errorf("stored ProjectID = %q, want proj_1", d.ProjectID)
	}
}

func TestHandleSetDatabaseProject_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/project", `{"project_id":"proj_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1", got.ProjectID)
	}
}

func TestHandleSetDatabaseProject_UnknownProject_Rejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/main/project", `{"project_id":"ghost"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetDatabaseProject_UnknownDatabase_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/databases/ghost/project", `{"project_id":""}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
