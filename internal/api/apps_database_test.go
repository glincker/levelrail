package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppDatabaseRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPut, "/api/v1/apps/web/database"},
		{http.MethodDelete, "/api/v1/apps/web/database"},
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

func TestHandleSetAppDatabase_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/missing/database", `{"database_name":"main"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSetAppDatabase_UnknownDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/database", `{"database_name":"missing"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppDatabase_MissingDatabaseName(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/database", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppDatabase_UnsupportedField(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "cache", Engine: store.EngineRedis}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/database", `{"database_name":"cache","field":"username"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppDatabase_Success_Defaults(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/database", `{"database_name":"main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appDatabaseResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := appDatabaseResource{AppName: "web", DatabaseName: "main", EnvVar: "DATABASE_URL", Field: "url"}
	if got != want {
		t.Errorf("response = %+v, want %+v", got, want)
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.DatabaseAttachment == nil || saved.DatabaseAttachment.DatabaseName != "main" || saved.DatabaseAttachment.EnvVar != "DATABASE_URL" || saved.DatabaseAttachment.Field != "url" {
		t.Errorf("persisted DatabaseAttachment = %+v, want database=main env_var=DATABASE_URL field=url", saved.DatabaseAttachment)
	}
}

// TestHandleGetApp_SurfacesDatabaseAttachment proves handleGetApp's
// existing response already carries the attachment once set, the same
// "no separate GET route" design TestHandleGetApp_SurfacesStorageTargetID
// already proves for storage_target_id.
func TestHandleGetApp_SurfacesDatabaseAttachment(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/database", `{"database_name":"main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("attach status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got appResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.DatabaseAttachment == nil || got.DatabaseAttachment.DatabaseName != "main" {
		t.Errorf("GET response DatabaseAttachment = %+v, want database_name=main", got.DatabaseAttachment)
	}
}

func TestHandleClearAppDatabase_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/missing/database", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleClearAppDatabase_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/database", `{"database_name":"main"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("attach status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/database", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.DatabaseAttachment != nil {
		t.Errorf("DatabaseAttachment = %+v, want nil after clear", saved.DatabaseAttachment)
	}
}
