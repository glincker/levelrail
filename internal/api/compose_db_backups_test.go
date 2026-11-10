package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeComposeDBRunner struct{ calls chan [4]string }

func (f *fakeComposeDBRunner) RunComposeServiceBackup(_ context.Context, _, app, service, engine, targetID string) error {
	f.calls <- [4]string{app, service, engine, targetID}
	return nil
}

func seedComposeApp(t *testing.T, db *store.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveApp(ctx, store.App{ID: "shop", Name: "shop", CreatedAt: "2026-11-08T00:00:00Z", UpdatedAt: "2026-11-08T00:00:00Z"}); err != nil {
		t.Fatalf("save app: %v", err)
	}
	for _, s := range []store.DesiredService{
		{Name: "shop-db", Image: "postgres:16", AppID: "shop"},
		{Name: "shop-web", Image: "nginx:1.27", AppID: "shop"},
	} {
		if err := db.SaveDesiredService(ctx, s); err != nil {
			t.Fatalf("save service %s: %v", s.Name, err)
		}
	}
}

func TestComposeDBBackupRoutes_RequireAuth(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/shop/compose-db-services"},
		{http.MethodPut, "/api/v1/apps/shop/compose-db-services/db/backup-schedule"},
		{http.MethodDelete, "/api/v1/apps/shop/compose-db-services/db/backup-schedule"},
		{http.MethodGet, "/api/v1/apps/shop/compose-db-services/db/backups"},
		{http.MethodPost, "/api/v1/apps/shop/compose-db-services/db/backups"},
	})
}

func TestComposeDBBackups_ScheduleLifecycleAndTrigger(t *testing.T) {
	t.Parallel()
	runner := &fakeComposeDBRunner{calls: make(chan [4]string, 2)}
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithComposeDBBackupRunner(runner))
	cookie := loginTestSession(t, rt, db)
	seedComposeApp(t, db)
	seedBackupTargetForAPI(t, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodGet, "/api/v1/apps/shop/compose-db-services", "")
	var listed []composeDBServiceResource
	if err := json.NewDecoder(rec.Body).Decode(&listed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("list: %d %v", rec.Code, err)
	}
	if len(listed) != 1 || listed[0].Service != "db" || listed[0].Engine != store.EnginePostgres || listed[0].Scheduled {
		t.Fatalf("detected = %+v, want only the postgres service, unscheduled", listed)
	}

	if rec = do(http.MethodPut, "/api/v1/apps/shop/compose-db-services/web/backup-schedule", `{"target_id":"bkt_test1","schedule":"0 3 * * *"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("non-db service PUT = %d, want 404", rec.Code)
	}
	if rec = do(http.MethodPut, "/api/v1/apps/shop/compose-db-services/db/backup-schedule", `{"target_id":"bkt_test1","schedule":"not cron"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad cron PUT = %d, want 400", rec.Code)
	}
	if rec = do(http.MethodPut, "/api/v1/apps/shop/compose-db-services/db/backup-schedule", `{"target_id":"bkt_test1","schedule":"0 3 * * *","retain":4}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
	}
	cfg, err := db.GetComposeServiceBackup(context.Background(), "shop", "db")
	if err != nil || cfg.BackupSchedule != "0 3 * * *" || cfg.BackupRetain != 4 || !cfg.Enabled || cfg.Engine != store.EnginePostgres {
		t.Fatalf("stored = %+v, %v", cfg, err)
	}

	if rec = do(http.MethodPost, "/api/v1/apps/shop/compose-db-services/db/backups", `{}`); rec.Code != http.StatusAccepted {
		t.Fatalf("trigger = %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case c := <-runner.calls:
		if c != [4]string{"shop", "db", store.EnginePostgres, "bkt_test1"} {
			t.Errorf("runner call = %v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner not called")
	}

	if rec = do(http.MethodGet, "/api/v1/apps/shop/compose-db-services/db/backups", ""); rec.Code != http.StatusOK {
		t.Fatalf("list runs = %d", rec.Code)
	}
	if rec = do(http.MethodDelete, "/api/v1/apps/shop/compose-db-services/db/backup-schedule", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if rec = do(http.MethodDelete, "/api/v1/apps/shop/compose-db-services/db/backup-schedule", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE = %d, want 404", rec.Code)
	}
}

func TestComposeDBBackups_TriggerWithoutRunner(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedComposeApp(t, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/shop/compose-db-services/db/backups", `{"target_id":"bkt_test1"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}
