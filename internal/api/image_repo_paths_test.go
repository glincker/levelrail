package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func squatRepo(t *testing.T, db *store.DB, repo string) {
	t.Helper()
	if _, err := db.ClaimImageRepo(context.Background(), normalizeImageRepo(repo), "holder"); err != nil {
		t.Fatalf("claim: %v", err)
	}
}

func TestImageRepoCreateRejectsOwnedRepo(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	squatRepo(t, db, "taken")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"taken","image":"nginx:1","port":80}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("create status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "taken"); err == nil {
		t.Fatal("app was saved despite the conflict")
	}
}

func TestImageRepoCloneRejectsOwnedRepo(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "src", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	squatRepo(t, db, "src-copy")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/src/clone", `{"new_name":"src-copy"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("clone status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImageRepoComposeRejectsOwnedRepo(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, n := range []string{"squat", "squat-s", "squat_s", "s"} {
		squatRepo(t, db, n)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/squat/compose", "services:\n  s:\n    image: alpine:3.20\n"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("compose status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}

func TestImageRepoImportAndSetImageRejectOwnedRepo(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	squatRepo(t, db, "imp")
	svc := store.DesiredService{Name: "imp", Image: "nginx:1", Port: 80}
	if st, _ := rt.checkServiceImageRepo(ctx, svc); st != http.StatusConflict {
		t.Fatalf("import gate status = %d, want 409", st)
	}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.setDesiredImage(ctx, svc, "imp:0123456789abcdef0123456789abcdef01234567"); err == nil {
		t.Fatal("setDesiredImage accepted an owned repo")
	}
}
