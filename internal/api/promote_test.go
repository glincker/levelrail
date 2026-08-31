package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPromoteRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodGet, "/api/v1/apps/web/promote/preview?to=env_1"},
		{http.MethodPost, "/api/v1/apps/web/promote"},
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

// seedPromotionFixture saves a project, an environment under it, and two
// apps tagged staging/production so every test below shares the same
// starting state.
func seedPromotionFixture(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web-staging", Image: "levelrail/web:2", Port: 3000}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web-staging", "proj_1"); err != nil {
		t.Fatalf("project source: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web-prod", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web-prod", "proj_1"); err != nil {
		t.Fatalf("project target: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "web-prod", "env_prod"); err != nil {
		t.Fatalf("tag target: %v", err)
	}
}

func TestHandlePromotePreview_AutoDiscoversSoleCandidate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web-staging/promote/preview?to=env_prod", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got promotePreviewResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TargetApp != "web-prod" {
		t.Errorf("TargetApp = %q, want web-prod", got.TargetApp)
	}
	if got.From.Image != "levelrail/web:2" || got.To.Image != "levelrail/web:1" {
		t.Errorf("From/To images = %q/%q, want levelrail/web:2/levelrail/web:1", got.From.Image, got.To.Image)
	}
	if len(got.Changes) != 1 || got.Changes[0].Field != "image" {
		t.Errorf("Changes = %+v, want one image change", got.Changes)
	}
	if len(got.UnsnapshottedFields) == 0 || got.Note == "" {
		t.Error("expected UnsnapshottedFields and Note to be populated")
	}
	// The target's own image must not have been touched by a preview.
	target, err := db.GetDesiredService(context.Background(), "web-prod")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if target.Image != "levelrail/web:1" {
		t.Errorf("preview must not mutate the target, Image = %q", target.Image)
	}
}

func TestHandlePromotePreview_ExplicitTarget(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web-staging/promote/preview?to=env_prod&target=web-prod", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandlePromotePreview_AmbiguousCandidates(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "worker-prod", Image: "levelrail/worker:1", Port: 4000}); err != nil {
		t.Fatalf("seed second target: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "worker-prod", "proj_1"); err != nil {
		t.Fatalf("project second target: %v", err)
	}
	if err := db.SetServiceEnvironment(ctx, "worker-prod", "env_prod"); err != nil {
		t.Fatalf("tag second target: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web-staging/promote/preview?to=env_prod", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandlePromotePreview_NoCandidates(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web-staging", Image: "levelrail/web:2", Port: 3000}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web-staging", "proj_1"); err != nil {
		t.Fatalf("project source: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web-staging/promote/preview?to=env_prod", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandlePromotePreview_SourceNotInSameProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "unrelated", Image: "levelrail/unrelated:1", Port: 5000}); err != nil {
		t.Fatalf("seed unrelated: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/unrelated/promote/preview?to=env_prod", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandlePromotePreview_UnknownEnvironment(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web-staging/promote/preview?to=env_ghost", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandlePromotePreview_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/promote/preview?to=env_prod", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandlePromoteApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)
	ctx := context.Background()

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got appResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "web-prod" || got.Image != "levelrail/web:2" {
		t.Errorf("response = name=%q image=%q, want web-prod/levelrail/web:2", got.Name, got.Image)
	}

	target, err := db.GetDesiredService(ctx, "web-prod")
	if err != nil {
		t.Fatalf("GetDesiredService: %v", err)
	}
	if target.Image != "levelrail/web:2" {
		t.Errorf("target Image = %q, want levelrail/web:2", target.Image)
	}
	// The source app's own image must be untouched by promoting from it.
	source, err := db.GetDesiredService(ctx, "web-staging")
	if err != nil {
		t.Fatalf("GetDesiredService(source): %v", err)
	}
	if source.Image != "levelrail/web:2" {
		t.Errorf("source Image changed to %q, want unchanged levelrail/web:2", source.Image)
	}

	attempts, err := db.ListDeployAttempts(ctx, "web-prod")
	if err != nil {
		t.Fatalf("ListDeployAttempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("len(attempts) = %d, want 1", len(attempts))
	}
	if attempts[0].Source != store.DeployAttemptSourcePromote {
		t.Errorf("attempt Source = %q, want %q", attempts[0].Source, store.DeployAttemptSourcePromote)
	}
	if attempts[0].Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("attempt Status = %q, want succeeded", attempts[0].Status)
	}
}

func TestHandlePromoteApp_TargetEqualsSource(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{"to":"env_prod","target":"web-staging"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandlePromoteApp_MissingTo(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPromotionFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web-staging/promote", `{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
