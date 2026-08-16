package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppStorageRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPut, "/api/v1/apps/web/storage"},
		{http.MethodDelete, "/api/v1/apps/web/storage"},
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

func TestHandleSetAppStorage_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveBackupTarget(context.Background(), store.BackupTarget{ID: "bkt_1", Name: "bucket", Provider: store.BackupProviderR2, Endpoint: "https://r2.example.com", Bucket: "data", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed backup target: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/missing/storage", `{"storage_target_id":"bkt_1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSetAppStorage_UnknownTarget(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/storage", `{"storage_target_id":"bkt_missing"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppStorage_MissingStorageTargetID(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/storage", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppStorage_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveBackupTarget(context.Background(), store.BackupTarget{ID: "bkt_1", Name: "bucket", Provider: store.BackupProviderR2, Endpoint: "https://r2.example.com", Bucket: "data", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed backup target: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/storage", `{"storage_target_id":"bkt_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got appStorageResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AppName != "web" || got.StorageTargetID != "bkt_1" {
		t.Errorf("response = %+v, want app_name=web storage_target_id=bkt_1", got)
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.StorageTargetID != "bkt_1" {
		t.Errorf("persisted StorageTargetID = %q, want bkt_1", saved.StorageTargetID)
	}
}

// TestHandleGetApp_SurfacesStorageTargetID proves the "no separate GET
// route" design decision (router.go's own comment on these routes):
// handleGetApp's existing response already carries storage_target_id
// once set.
func TestHandleGetApp_SurfacesStorageTargetID(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveBackupTarget(context.Background(), store.BackupTarget{ID: "bkt_1", Name: "bucket", Provider: store.BackupProviderR2, Endpoint: "https://r2.example.com", Bucket: "data", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed backup target: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/storage", `{"storage_target_id":"bkt_1"}`))
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
	if got.StorageTargetID != "bkt_1" {
		t.Errorf("GET response StorageTargetID = %q, want bkt_1", got.StorageTargetID)
	}
}

func TestHandleClearAppStorage_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/missing/storage", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleClearAppStorage_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SaveBackupTarget(context.Background(), store.BackupTarget{ID: "bkt_1", Name: "bucket", Provider: store.BackupProviderR2, Endpoint: "https://r2.example.com", Bucket: "data", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("seed backup target: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/storage", `{"storage_target_id":"bkt_1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("attach status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/storage", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	saved, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if saved.StorageTargetID != "" {
		t.Errorf("StorageTargetID = %q, want empty after clear", saved.StorageTargetID)
	}
}
