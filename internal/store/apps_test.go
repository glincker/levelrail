package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func newTestApp() App {
	return App{
		ID:        "app_test1",
		Name:      "my-app",
		CreatedAt: "2026-08-14T00:00:00Z",
		UpdatedAt: "2026-08-14T00:00:00Z",
	}
}

func TestSaveAndGetApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestApp()
	if err := db.SaveApp(ctx, want); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	got, err := db.GetApp(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetApp() error = %v", err)
	}
	if got != want {
		t.Errorf("GetApp() = %+v, want %+v", got, want)
	}
}

func TestGetApp_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetApp(ctx, "app_missing")
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("GetApp() error = %v, want ErrAppNotFound", err)
	}
}

func TestGetAppByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestApp()
	if err := db.SaveApp(ctx, want); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}

	got, err := db.GetAppByName(ctx, want.Name)
	if err != nil {
		t.Fatalf("GetAppByName() error = %v", err)
	}
	if got != want {
		t.Errorf("GetAppByName() = %+v, want %+v", got, want)
	}
}

func TestGetAppByName_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetAppByName(ctx, "does-not-exist")
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("GetAppByName() error = %v, want ErrAppNotFound", err)
	}
}

func TestSaveApp_UpsertsOnConflict(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	a := newTestApp()
	if err := db.SaveApp(ctx, a); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}
	a.Name = "renamed-app"
	a.UpdatedAt = "2026-08-14T01:00:00Z"
	if err := db.SaveApp(ctx, a); err != nil {
		t.Fatalf("SaveApp() (update) error = %v", err)
	}

	got, err := db.GetApp(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetApp() error = %v", err)
	}
	if got.Name != "renamed-app" {
		t.Errorf("Name = %q after upsert, want %q", got.Name, "renamed-app")
	}
}

func TestListApps_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	b := App{ID: "app_b", Name: "b-app", CreatedAt: "2026-08-14T00:00:00Z", UpdatedAt: "2026-08-14T00:00:00Z"}
	a := App{ID: "app_a", Name: "a-app", CreatedAt: "2026-08-14T00:00:01Z", UpdatedAt: "2026-08-14T00:00:01Z"}
	if err := db.SaveApp(ctx, b); err != nil {
		t.Fatalf("SaveApp(b) error = %v", err)
	}
	if err := db.SaveApp(ctx, a); err != nil {
		t.Fatalf("SaveApp(a) error = %v", err)
	}

	got, err := db.ListApps(ctx)
	if err != nil {
		t.Fatalf("ListApps() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "a-app" || got[1].Name != "b-app" {
		t.Fatalf("ListApps() = %+v, want [a-app, b-app] in name order", got)
	}
}

func TestDeleteApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestApp()
	if err := db.SaveApp(ctx, want); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}
	if err := db.DeleteApp(ctx, want.ID); err != nil {
		t.Fatalf("DeleteApp() error = %v", err)
	}

	_, err := db.GetApp(ctx, want.ID)
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("GetApp() after delete error = %v, want ErrAppNotFound", err)
	}
}

func TestDeleteApp_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteApp(ctx, "app_missing")
	if !errors.Is(err, ErrAppNotFound) {
		t.Fatalf("DeleteApp() error = %v, want ErrAppNotFound", err)
	}
}

// linkServiceToApp is a test-only helper: stage 1 has no store setter
// for desired_services.app_id (see DesiredService.AppID's own doc
// comment), so tests wire it directly the way a later stage's own
// setter eventually will.
func linkServiceToApp(t *testing.T, db *DB, serviceName, appID string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `UPDATE desired_services SET app_id = ? WHERE name = ?`, appID, serviceName); err != nil {
		t.Fatalf("link service %q to app %q: %v", serviceName, appID, err)
	}
}

// seedAppWithWebWorker saves app plus a "web"/"worker" service pair
// linked to it, the shared fixture TestDeleteApp_CascadesToServices and
// TestListServicesByApp both build on.
func seedAppWithWebWorker(t *testing.T, db *DB, app App) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveApp(ctx, app); err != nil {
		t.Fatalf("SaveApp() error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "worker", Image: "img:v1", Port: 9090}); err != nil {
		t.Fatalf("SaveDesiredService(worker) error = %v", err)
	}
	linkServiceToApp(t, db, "web", app.ID)
	linkServiceToApp(t, db, "worker", app.ID)
}

// TestDeleteApp_CascadesToServices is the real regression
// desired_services.app_id's "ON DELETE CASCADE" foreign key
// (migrations/0039_apps.sql) exists to guarantee, the mirror image of
// TestDeleteProject_ServiceBecomesProjectLess's SET NULL behavior: an
// app owns its services' lifecycle, so deleting it must delete every
// service that belonged to it, not leave them orphaned.
func TestDeleteApp_CascadesToServices(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	app := newTestApp()
	seedAppWithWebWorker(t, db, app)

	if err := db.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp() error = %v", err)
	}

	if _, err := db.GetDesiredService(ctx, "web"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("GetDesiredService(web) after app delete error = %v, want ErrServiceNotFound (cascade should have removed it)", err)
	}
	if _, err := db.GetDesiredService(ctx, "worker"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("GetDesiredService(worker) after app delete error = %v, want ErrServiceNotFound (cascade should have removed it)", err)
	}
}

func TestListServicesByApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	app := newTestApp()
	seedAppWithWebWorker(t, db, app)
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "unrelated", Image: "img:v1", Port: 7070}); err != nil {
		t.Fatalf("SaveDesiredService(unrelated) error = %v", err)
	}

	got, err := db.ListServicesByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListServicesByApp() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "web" || got[1].Name != "worker" {
		t.Fatalf("ListServicesByApp() = %+v, want [web, worker]", got)
	}
	for _, svc := range got {
		if svc.AppID != app.ID {
			t.Errorf("service %q AppID = %q, want %q", svc.Name, svc.AppID, app.ID)
		}
	}
}

func TestListServicesByApp_NoMatches(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.ListServicesByApp(ctx, "app_missing")
	if err != nil {
		t.Fatalf("ListServicesByApp() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListServicesByApp() = %+v, want empty", got)
	}
}

// TestAppsMigration_BackfillsExistingServices applies every migration
// except the apps one by hand, inserts a desired_services row the way a
// pre-apps-migration database would have one, then applies the apps
// migration and checks the backfill: the founder-approved design
// requires this to be pure SQL inside the migration itself, not an
// application-layer step, so this test exercises the migration file
// directly rather than
// through store.Open (which would apply every migration in one pass,
// leaving nothing to backfill).
func TestAppsMigration_BackfillsExistingServices(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "levelrail.db")

	sqlDB, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{DB: sqlDB}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}

	// Matched by name, not a hardcoded version number: this file has
	// already been renumbered once (0030 -> 0037) as other concurrently
	// merged migrations claimed 0030-0036, and a hardcoded version here
	// would silently start testing the wrong migration on the next
	// renumber instead of failing loudly.
	var appsMigration *migration
	for i, m := range migrations {
		if m.name == "apps" {
			appsMigration = &migrations[i]
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			t.Fatalf("apply migration %04d_%s: %v", m.version, m.name, err)
		}
	}
	if appsMigration == nil {
		t.Fatal("apps migration not found among embedded migrations")
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO desired_services (name, image, port, updated_at)
		VALUES ('legacy-web', 'img:v1', 8080, '2026-08-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert pre-migration desired_services row: %v", err)
	}

	if err := db.applyMigration(ctx, *appsMigration); err != nil {
		t.Fatalf("apply apps migration: %v", err)
	}

	app, err := db.GetAppByName(ctx, "legacy-web")
	if err != nil {
		t.Fatalf("GetAppByName(legacy-web) after backfill error = %v", err)
	}
	if app.ID != "legacy-web" {
		t.Errorf("backfilled app.ID = %q, want %q (id = name for a backfilled row)", app.ID, "legacy-web")
	}

	svc, err := db.GetDesiredService(ctx, "legacy-web")
	if err != nil {
		t.Fatalf("GetDesiredService(legacy-web) error = %v", err)
	}
	if svc.AppID != app.ID {
		t.Errorf("service.AppID = %q after backfill, want %q", svc.AppID, app.ID)
	}
}
