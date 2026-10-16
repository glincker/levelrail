package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedStaticTree(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name, "abc")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedStaticSite(t *testing.T, db *store.DB, root, name string, domains ...string) {
	t.Helper()
	seedStaticTree(t, root, name)
	site := store.StaticSite{Name: name, Domains: domains, RootDir: filepath.Join(root, name, "abc")}
	if err := db.SaveStaticSite(context.Background(), site); err != nil {
		t.Fatal(err)
	}
}

func ageStaticSite(t *testing.T, db *store.DB, name string, age time.Duration) {
	t.Helper()
	ts := time.Now().UTC().Add(-age).Format("2006-01-02T15:04:05.000Z")
	if _, err := db.ExecContext(context.Background(), `UPDATE static_sites SET updated_at = ? WHERE name = ?`, ts, name); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteApp_CascadesStaticSite(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	root := t.TempDir()
	rt.staticRootDir = root
	seedApp(t, db, "web")
	seedStaticSite(t, db, root, "web", "web.example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete app = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetStaticSite(context.Background(), "web"); err == nil {
		t.Error("static site row survived app delete")
	}
	if _, err := os.Stat(filepath.Join(root, "web")); !os.IsNotExist(err) {
		t.Errorf("published tree survived app delete: %v", err)
	}
	var n int
	if err := db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM static_site_domains`).Scan(&n); err != nil || n != 0 {
		t.Errorf("domain claims left = %d, err = %v", n, err)
	}
}

func TestHandleDeleteStaticSite(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	root := t.TempDir()
	rt.staticRootDir = root
	seedStaticSite(t, db, root, "orphan", "o.example.com")
	seedStaticSite(t, db, root, "live")
	seedApp(t, db, "live")

	do := func(path string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, path, ""))
		return rec.Code
	}
	if got := do("/api/v1/static-sites/missing"); got != http.StatusNotFound {
		t.Errorf("missing = %d, want 404", got)
	}
	if got := do("/api/v1/static-sites/live"); got != http.StatusConflict {
		t.Errorf("site with app = %d, want 409", got)
	}
	if got := do("/api/v1/static-sites/orphan"); got != http.StatusNoContent {
		t.Fatalf("orphan = %d, want 204", got)
	}
	if _, err := db.GetStaticSite(context.Background(), "orphan"); err == nil {
		t.Error("row survived delete")
	}
	if _, err := os.Stat(filepath.Join(root, "orphan")); !os.IsNotExist(err) {
		t.Errorf("tree survived delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "live")); err != nil {
		t.Errorf("unrelated tree touched: %v", err)
	}
}

func TestHandleDeleteStaticSite_ReadOnlyTokenForbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	tok := seedMatrixToken(t, db, "ro-static", []string{AbilityRead})
	if got := matrixDo(rt, http.MethodDelete, "/api/v1/static-sites/x", bearer(tok)); got != http.StatusForbidden {
		t.Errorf("read-only token = %d, want 403", got)
	}
	if got := matrixDo(rt, http.MethodDelete, "/api/v1/static-sites/x", nil); got != http.StatusUnauthorized {
		t.Errorf("anonymous = %d, want 401", got)
	}
}

func TestSweepOrphanStaticSites(t *testing.T) {
	rt, db := newTestRouter(t)
	root := t.TempDir()
	rt.staticRootDir = root
	rt.staticOrphanGrace = 10 * time.Minute

	seedStaticSite(t, db, root, "fresh")
	seedStaticSite(t, db, root, "old-orphan")
	seedStaticSite(t, db, root, "old-owned")
	seedStaticSite(t, db, root, "old-pending")
	seedApp(t, db, "old-owned")
	for _, n := range []string{"old-orphan", "old-owned", "old-pending"} {
		ageStaticSite(t, db, n, time.Hour)
	}
	if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: "dep1", ServiceName: "old-pending", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := rt.SweepOrphanStaticSites(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "old-orphan" {
		t.Fatalf("removed = %v, want [old-orphan]", removed)
	}
	for _, n := range []string{"fresh", "old-owned", "old-pending"} {
		if _, err := db.GetStaticSite(context.Background(), n); err != nil {
			t.Errorf("site %q wrongly removed: %v", n, err)
		}
		if _, err := os.Stat(filepath.Join(root, n)); err != nil {
			t.Errorf("tree %q wrongly removed: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "old-orphan")); !os.IsNotExist(err) {
		t.Errorf("orphan tree survived: %v", err)
	}
}
