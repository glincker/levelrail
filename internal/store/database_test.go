package store

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func TestSaveAndGetDesiredDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}
	if err := db.SaveDesiredDatabase(ctx, want); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if *got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestSaveAndGetDesiredDatabase_MySQL reproduces the real bug migration
// 0016 fixes: desired_databases.engine originally carried a hardcoded
// CHECK (engine IN ('postgres', 'redis')) (0003_desired_databases.sql),
// so a mysql row failed at the SQLite layer even though every
// application-layer check (validateDatabaseResource,
// SupportedDatabaseEngines) already accepted it. Confirmed live via a
// real browser click-through before this test existed: "CHECK
// constraint failed: engine IN ('postgres', 'redis')".
func TestSaveAndGetDesiredDatabase_MySQL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredDatabase{Name: "orders", Engine: EngineMySQL, Version: "8"}
	if err := db.SaveDesiredDatabase(ctx, want); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "orders")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if *got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSaveDesiredDatabase_UpsertReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "15"}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.Version != "16" {
		t.Errorf("Version = %q, want 16 (the second save's value)", got.Version)
	}

	all, err := db.ListDesiredDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDesiredDatabases() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected exactly 1 row after two saves to the same name, got %d", len(all))
	}
}

func TestGetDesiredDatabase_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDesiredDatabase(context.Background(), "never-saved")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestListDesiredDatabases_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"cache", "main", "analytics"} {
		if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: name, Engine: EngineRedis}); err != nil {
			t.Fatalf("save %q: %v", name, err)
		}
	}

	got, err := db.ListDesiredDatabases(ctx)
	if err != nil {
		t.Fatalf("ListDesiredDatabases() error = %v", err)
	}
	want := []string{"analytics", "cache", "main"}
	if len(got) != len(want) {
		t.Fatalf("expected %d databases, got %d", len(want), len(got))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: got %q, want %q (expected alphabetical order)", i, got[i].Name, name)
		}
	}
}

func TestUpdateDatabaseNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node-1"); err != nil {
		t.Fatalf("UpdateDatabaseNode() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1", got.NodeID)
	}
}

func TestUpdateDatabaseNode_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateDatabaseNode(context.Background(), "nonexistent", "node-1")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("UpdateDatabaseNode() error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestSaveDesiredDatabase_ResaveDoesNotResetNodeID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "15"}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node-1"); err != nil {
		t.Fatalf("UpdateDatabaseNode() error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("resave: %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.Version != "16" {
		t.Errorf("Version = %q, want 16", got.Version)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1 (a resave must not silently un-assign a placed database)", got.NodeID)
	}
}

func TestUpdateDatabaseProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.UpdateDatabaseProject(ctx, "main", "proj_1"); err != nil {
		t.Fatalf("UpdateDatabaseProject() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1", got.ProjectID)
	}
}

func TestUpdateDatabaseProject_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateDatabaseProject(context.Background(), "nonexistent", "proj_1")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("UpdateDatabaseProject() error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestSaveDesiredDatabase_ResaveDoesNotResetProjectID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "15"}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.UpdateDatabaseProject(ctx, "main", "proj_1"); err != nil {
		t.Fatalf("UpdateDatabaseProject() error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("resave: %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.Version != "16" {
		t.Errorf("Version = %q, want 16", got.Version)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1 (a resave must not silently un-assign a project)", got.ProjectID)
	}
}

func TestDeleteDesiredDatabase(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.DeleteDesiredDatabase(ctx, "main"); err != nil {
		t.Fatalf("DeleteDesiredDatabase() error = %v", err)
	}

	if _, err := db.GetDesiredDatabase(ctx, "main"); !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("GetDesiredDatabase after delete: error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestDeleteDesiredDatabase_NotFound(t *testing.T) {
	db := openTestDB(t)

	err := db.DeleteDesiredDatabase(context.Background(), "never-saved")
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("error = %v, want ErrDatabaseNotFound", err)
	}
}

func TestDeleteDesiredDatabase_DoesNotAffectOtherDatabases(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"cache", "main"} {
		if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: name, Engine: EngineRedis, Version: "7"}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}

	if err := db.DeleteDesiredDatabase(ctx, "main"); err != nil {
		t.Fatalf("DeleteDesiredDatabase() error = %v", err)
	}

	if _, err := db.GetDesiredDatabase(ctx, "cache"); err != nil {
		t.Errorf("unrelated database cache should be untouched, got error = %v", err)
	}
}

func TestSetDatabaseBackupSchedule(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	target := seedBackupTarget(t, db)

	if err := db.SetDatabaseBackupSchedule(ctx, "main", target.ID, "0 3 * * *", 7); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.BackupTargetID != target.ID {
		t.Errorf("BackupTargetID = %q, want %q", got.BackupTargetID, target.ID)
	}
	if got.BackupSchedule != "0 3 * * *" {
		t.Errorf("BackupSchedule = %q, want %q", got.BackupSchedule, "0 3 * * *")
	}
	if got.BackupRetain != 7 {
		t.Errorf("BackupRetain = %d, want 7", got.BackupRetain)
	}
}

func TestSetDatabaseBackupSchedule_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.SetDatabaseBackupSchedule(context.Background(), "nonexistent", "bkt_1", "0 3 * * *", 7)
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("SetDatabaseBackupSchedule() error = %v, want ErrDatabaseNotFound", err)
	}
}

// TestSetDatabaseBackupSchedule_EmptyClears is the DELETE
// /api/v1/databases/{name}/backup-schedule path: passing empty targetID
// and schedule (retain forced to 0 by the caller) must return the
// database to its pre-scheduled, zero-value state, not merely leave a
// dangling schedule string with no target.
func TestSetDatabaseBackupSchedule_EmptyClears(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	target := seedBackupTarget(t, db)
	if err := db.SetDatabaseBackupSchedule(ctx, "main", target.ID, "0 3 * * *", 7); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}

	if err := db.SetDatabaseBackupSchedule(ctx, "main", "", "", 0); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule(clear) error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.BackupTargetID != "" || got.BackupSchedule != "" || got.BackupRetain != 0 {
		t.Errorf("after clear, got %+v, want all three fields zero-valued", got)
	}
}

// TestSetDatabaseBackupSchedule_TargetDeletedClearsColumn exercises
// migrations/0023's own ON DELETE SET NULL: deleting a backup_targets
// row a schedule points at must clear backup_target_id automatically,
// not leave it pointing at nothing.
func TestSetDatabaseBackupSchedule_TargetDeletedClearsColumn(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	target := seedBackupTarget(t, db)
	if err := db.SetDatabaseBackupSchedule(ctx, "main", target.ID, "0 3 * * *", 7); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}

	if err := db.DeleteBackupTarget(ctx, target.ID); err != nil {
		t.Fatalf("DeleteBackupTarget() error = %v", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.BackupTargetID != "" {
		t.Errorf("BackupTargetID = %q after target delete, want empty (ON DELETE SET NULL)", got.BackupTargetID)
	}
	// backup_schedule is untouched by the FK action: only the target
	// reference is cleared, see SetDatabaseBackupSchedule's own doc
	// comment and internal/backup.Scheduler's own handling of this
	// half-configured state.
	if got.BackupSchedule != "0 3 * * *" {
		t.Errorf("BackupSchedule = %q after target delete, want it untouched", got.BackupSchedule)
	}
}

func TestListScheduledDatabases(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	for _, name := range []string{"scheduled", "unscheduled"} {
		if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: name, Engine: EnginePostgres, Version: "16"}); err != nil {
			t.Fatalf("SaveDesiredDatabase(%s) error = %v", name, err)
		}
	}
	if err := db.SetDatabaseBackupSchedule(ctx, "scheduled", target.ID, "0 3 * * *", 7); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}

	got, err := db.ListScheduledDatabases(ctx)
	if err != nil {
		t.Fatalf("ListScheduledDatabases() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "scheduled" {
		t.Fatalf("ListScheduledDatabases() = %+v, want exactly [scheduled]", got)
	}
}

// TestListScheduledDatabases_ExcludesHalfConfigured covers the exact
// scenario ListScheduledDatabases's own doc comment describes: a
// schedule string survives its target's deletion (the FK action only
// clears backup_target_id, see TestSetDatabaseBackupSchedule_
// TargetDeletedClearsColumn above), and that half-configured row must
// not show up as "scheduled" since it can never actually run.
func TestListScheduledDatabases_ExcludesHalfConfigured(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	target := seedBackupTarget(t, db)

	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.SetDatabaseBackupSchedule(ctx, "main", target.ID, "0 3 * * *", 7); err != nil {
		t.Fatalf("SetDatabaseBackupSchedule() error = %v", err)
	}
	if err := db.DeleteBackupTarget(ctx, target.ID); err != nil {
		t.Fatalf("DeleteBackupTarget() error = %v", err)
	}

	got, err := db.ListScheduledDatabases(ctx)
	if err != nil {
		t.Fatalf("ListScheduledDatabases() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("ListScheduledDatabases() = %+v, want empty (target deleted, schedule half-configured)", got)
	}
}

// TestListDesiredDatabasesByNode is the database-kind counterpart to
// TestListDesiredServicesByNode (service_test.go), same TASKS.md 3.7
// drain/delete-guard callers.
func TestListDesiredDatabasesByNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, d := range []DesiredDatabase{
		{Name: "main", Engine: EngineRedis, Version: "7"},
		{Name: "cache", Engine: EngineRedis, Version: "7"},
	} {
		if err := db.SaveDesiredDatabase(ctx, d); err != nil {
			t.Fatalf("SaveDesiredDatabase(%s) error = %v", d.Name, err)
		}
	}
	if err := db.UpdateDatabaseNode(ctx, "main", "node-1"); err != nil {
		t.Fatalf("UpdateDatabaseNode(main) error = %v", err)
	}
	// cache stays on the local node ("").

	got, err := db.ListDesiredDatabasesByNode(ctx, "node-1")
	if err != nil {
		t.Fatalf("ListDesiredDatabasesByNode(node-1) error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "main" {
		t.Fatalf("ListDesiredDatabasesByNode(node-1) = %+v, want [main]", got)
	}

	gotEmpty, err := db.ListDesiredDatabasesByNode(ctx, "node-2")
	if err != nil {
		t.Fatalf("ListDesiredDatabasesByNode(node-2) error = %v", err)
	}
	if len(gotEmpty) != 0 {
		t.Errorf("ListDesiredDatabasesByNode(node-2) = %+v, want empty", gotEmpty)
	}
}

func TestSetDatabasePublicAccess_AutoAssignsFromRange(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	port, err := db.SetDatabasePublicAccess(ctx, "main", true, 0)
	if err != nil {
		t.Fatalf("SetDatabasePublicAccess() error = %v", err)
	}
	if port < PublicPortRangeStart || port > PublicPortRangeEnd {
		t.Errorf("assigned port %d, want in [%d, %d]", port, PublicPortRangeStart, PublicPortRangeEnd)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if !got.PubliclyAccessible || got.PublicPort != port {
		t.Errorf("got PubliclyAccessible=%v PublicPort=%d, want true %d", got.PubliclyAccessible, got.PublicPort, port)
	}
}

func TestSetDatabasePublicAccess_ExplicitPort(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}

	port, err := db.SetDatabasePublicAccess(ctx, "main", true, 15432)
	if err != nil {
		t.Fatalf("SetDatabasePublicAccess() error = %v", err)
	}
	if port != 15432 {
		t.Errorf("port = %d, want 15432", port)
	}
}

// TestSetDatabasePublicAccess_ExplicitPortAlreadyTaken proves the
// friendly ErrPublicPortInUse path fires before the unique index ever
// would: two databases requesting the identical explicit port, second
// one must be rejected, first one's assignment untouched.
func TestSetDatabasePublicAccess_ExplicitPortAlreadyTaken(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, name := range []string{"main", "cache"} {
		if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: name, Engine: EngineRedis, Version: "7"}); err != nil {
			t.Fatalf("SaveDesiredDatabase(%s) error = %v", name, err)
		}
	}
	if _, err := db.SetDatabasePublicAccess(ctx, "main", true, 15432); err != nil {
		t.Fatalf("SetDatabasePublicAccess(main) error = %v", err)
	}

	_, err := db.SetDatabasePublicAccess(ctx, "cache", true, 15432)
	if !errors.Is(err, ErrPublicPortInUse) {
		t.Errorf("SetDatabasePublicAccess(cache) error = %v, want ErrPublicPortInUse", err)
	}

	got, err := db.GetDesiredDatabase(ctx, "cache")
	if err != nil {
		t.Fatalf("GetDesiredDatabase(cache) error = %v", err)
	}
	if got.PubliclyAccessible {
		t.Errorf("cache PubliclyAccessible = true, want false: the rejected claim must not have been written")
	}
}

// TestSetDatabasePublicAccess_ReenablingSamePortDoesNotConflictWithSelf
// proves claimPublicPort's own name-exclusion: re-running the same
// enable call for a database that already holds a port (e.g. a repeated
// PUT, or SaveDesiredDatabase's own upsert semantics being exercised
// again) must not treat the database's own prior claim as taken.
func TestSetDatabasePublicAccess_ReenablingSamePortDoesNotConflictWithSelf(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if _, err := db.SetDatabasePublicAccess(ctx, "main", true, 15432); err != nil {
		t.Fatalf("first SetDatabasePublicAccess() error = %v", err)
	}

	port, err := db.SetDatabasePublicAccess(ctx, "main", true, 15432)
	if err != nil {
		t.Fatalf("second SetDatabasePublicAccess() error = %v", err)
	}
	if port != 15432 {
		t.Errorf("port = %d, want 15432", port)
	}
}

func TestSetDatabasePublicAccess_DisableClearsPort(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if _, err := db.SetDatabasePublicAccess(ctx, "main", true, 15432); err != nil {
		t.Fatalf("enable SetDatabasePublicAccess() error = %v", err)
	}

	port, err := db.SetDatabasePublicAccess(ctx, "main", false, 0)
	if err != nil {
		t.Fatalf("disable SetDatabasePublicAccess() error = %v", err)
	}
	if port != 0 {
		t.Errorf("disable returned port %d, want 0", port)
	}

	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase() error = %v", err)
	}
	if got.PubliclyAccessible || got.PublicPort != 0 {
		t.Errorf("after disable, got PubliclyAccessible=%v PublicPort=%d, want false 0", got.PubliclyAccessible, got.PublicPort)
	}
}

func TestSetDatabasePublicAccess_NotFound(t *testing.T) {
	_, err := openTestDB(t).SetDatabasePublicAccess(context.Background(), "missing", true, 0)
	if !errors.Is(err, ErrDatabaseNotFound) {
		t.Errorf("SetDatabasePublicAccess() error = %v, want ErrDatabaseNotFound", err)
	}
}

// TestSetDatabasePublicAccess_RangeExhausted proves
// ErrPublicPortRangeExhausted actually fires once every port in range is
// claimed, exercised against a tiny synthetic range rather than filling
// the real 1000-port default one row at a time.
func TestSetDatabasePublicAccess_RangeExhausted(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "main", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, DesiredDatabase{Name: "other", Engine: EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase() error = %v", err)
	}
	// Claim every port in the real range via direct SQL, cheaper than
	// PublicPortRangeEnd-PublicPortRangeStart+1 individual API calls:
	// this test only needs claimPublicPort to see the range fully taken,
	// not to exercise SetDatabasePublicAccess's own commit path for each
	// one.
	if _, err := db.ExecContext(ctx, `UPDATE desired_databases SET publicly_accessible = 1, public_port = ? WHERE name = 'other'`, PublicPortRangeStart); err != nil {
		t.Fatalf("seed other's port: %v", err)
	}
	for p := PublicPortRangeStart + 1; p <= PublicPortRangeEnd; p++ {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO desired_databases (name, engine, version, node_id, publicly_accessible, public_port, updated_at)
			VALUES (?, 'redis', '7', '', 1, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		`, "filler-"+strconv.Itoa(p), p); err != nil {
			t.Fatalf("seed filler port %d: %v", p, err)
		}
	}

	_, err := db.SetDatabasePublicAccess(ctx, "main", true, 0)
	if !errors.Is(err, ErrPublicPortRangeExhausted) {
		t.Errorf("SetDatabasePublicAccess() error = %v, want ErrPublicPortRangeExhausted", err)
	}
}
