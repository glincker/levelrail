package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedCascade(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.SaveOrganization(ctx, store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-08-14T00:00:00Z"}))
	must(db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "saas", CreatedAt: "2026-08-14T00:00:00Z"}))
	must(db.SetProjectOrganization(ctx, "proj_1", "org_1"))
	must(db.SaveEnvironment(ctx, store.Environment{ID: "env_1", ProjectID: "proj_1", Name: "staging"}))
	must(db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "i:1", Port: 80}))
	must(db.UpdateServiceProject(ctx, "web", "proj_1"))
	must(db.SaveDesiredService(ctx, store.DesiredService{Name: "worker", Image: "i:1", Port: 80}))
	must(db.SetServiceEnvironment(ctx, "worker", "env_1"))
	must(db.SaveDesiredService(ctx, store.DesiredService{Name: "other", Image: "i:1", Port: 80}))
}

func TestCascadeDelete(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantApps   []string
		wantGone   func(*store.DB) bool
		wantKeepOn []string
	}{
		{"environment", "/api/v1/environments/env_1?cascade=true", []string{"worker"}, func(db *store.DB) bool {
			_, err := db.GetEnvironment(context.Background(), "env_1")
			return err != nil
		}, []string{"web", "other"}},
		{"project", "/api/v1/projects/proj_1?cascade=true", []string{"web", "worker"}, func(db *store.DB) bool {
			_, err := db.GetProject(context.Background(), "proj_1")
			return err != nil
		}, []string{"other"}},
		{"organization", "/api/v1/organizations/org_1?cascade=true", []string{"web", "worker"}, func(db *store.DB) bool {
			_, perr := db.GetProject(context.Background(), "proj_1")
			_, oerr := db.GetOrganization(context.Background(), "org_1")
			return perr != nil && oerr != nil
		}, []string{"other"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			seedCascade(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, tc.target, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var resp cascadeResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if len(resp.DeletedApps) != len(tc.wantApps) {
				t.Fatalf("deleted = %v, want %v", resp.DeletedApps, tc.wantApps)
			}
			if !tc.wantGone(db) {
				t.Fatal("the container itself was not deleted")
			}
			for _, n := range tc.wantApps {
				if _, err := db.GetDesiredService(context.Background(), n); err == nil {
					t.Fatalf("app %q survived the cascade", n)
				}
			}
			for _, n := range tc.wantKeepOn {
				if _, err := db.GetDesiredService(context.Background(), n); err != nil {
					t.Fatalf("app %q outside the scope was deleted: %v", n, err)
				}
			}
		})
	}
}

func TestCascadeDelete_KeepsDatabaseUsedOutsideScopeAndResumes(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedCascade(t, db)
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseProject(ctx, "main", "proj_1"); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceDatabaseAttachment(ctx, "other", &store.DatabaseAttachment{DatabaseName: "main", EnvVar: "DATABASE_URL", Field: "url"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/projects/proj_1?cascade=true", ""))
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetProject(ctx, "proj_1"); err != nil {
		t.Fatal("project removed while a member is still held back")
	}
	if _, err := db.GetDesiredService(ctx, "web"); err == nil {
		t.Fatal("app was not deleted on the partial pass")
	}

	if err := db.DeleteDesiredService(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/projects/proj_1?cascade=true", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetProject(ctx, "proj_1"); err == nil {
		t.Fatal("project still exists after a clean resume")
	}
}

func TestDelete_WithoutCascadeStillDetaches(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCascade(t, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/organizations/org_1", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err != nil {
		t.Fatal("app deleted without cascade")
	}
}
