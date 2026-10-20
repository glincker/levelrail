package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDBUpgradeRunLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	run := DBUpgradeRun{
		ID: "dbu_1", DatabaseName: "main", Engine: EnginePostgres, FromVersion: "16.4", ToVersion: "16.11",
		Kind: "patch", Source: DBUpgradeSourceAuto, State: DBUpgradeStatePending, VerifyAfter: true,
		RevertOnFailure: true, Notify: []string{"chn_a"}, CreatedAt: "2026-10-05T00:00:00Z",
	}
	if err := db.CreateDBUpgradeRun(ctx, run); err != nil {
		t.Fatalf("CreateDBUpgradeRun() error = %v", err)
	}
	active, err := db.ListActiveDBUpgradeRuns(ctx)
	if err != nil || len(active) != 1 {
		t.Fatalf("ListActiveDBUpgradeRuns() = %v, %v, want one", active, err)
	}

	run.State = DBUpgradeStateReverted
	run.RevertPath = "image"
	run.Reason = "health check failed"
	run.Timings = map[string]string{"pending": "2026-10-05T00:00:00Z"}
	run.FinishedAt = "2026-10-05T00:10:00Z"
	if err := db.UpdateDBUpgradeRun(ctx, run); err != nil {
		t.Fatalf("UpdateDBUpgradeRun() error = %v", err)
	}
	got, err := db.GetDBUpgradeRun(ctx, "dbu_1")
	if err != nil {
		t.Fatalf("GetDBUpgradeRun() error = %v", err)
	}
	if got.State != DBUpgradeStateReverted || got.RevertPath != "image" || !got.VerifyAfter || len(got.Notify) != 1 || got.Timings["pending"] == "" {
		t.Errorf("got %+v", got)
	}
	if active, _ := db.ListActiveDBUpgradeRuns(ctx); len(active) != 0 {
		t.Errorf("active after finish = %d, want 0", len(active))
	}
	if list, _ := db.ListDBUpgradeRuns(ctx, "main", 10); len(list) != 1 {
		t.Errorf("ListDBUpgradeRuns() = %d rows, want 1", len(list))
	}
	if _, err := db.GetDBUpgradeRun(ctx, "nope"); !errors.Is(err, ErrDBUpgradeRunNotFound) {
		t.Errorf("GetDBUpgradeRun(missing) error = %v", err)
	}
	if err := db.UpdateDBUpgradeRun(ctx, DBUpgradeRun{ID: "nope", State: DBUpgradeStateFailed}); !errors.Is(err, ErrDBUpgradeRunNotFound) {
		t.Errorf("UpdateDBUpgradeRun(missing) error = %v", err)
	}
}

func TestDBUpgradeRunRejectsUnknownState(t *testing.T) {
	db := openTestDB(t)
	err := db.CreateDBUpgradeRun(context.Background(), DBUpgradeRun{
		ID: "dbu_x", DatabaseName: "main", Engine: EnginePostgres, FromVersion: "16", ToVersion: "16.1",
		Kind: "patch", Source: DBUpgradeSourceAuto, State: "reverting", CreatedAt: "2026-10-05T00:00:00Z",
	})
	if err == nil {
		t.Fatal("CreateDBUpgradeRun() with an unknown state succeeded, want a CHECK failure")
	}
}

func TestDBUpgradePolicyDefaultAndOverride(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	def, found, err := db.GetDBUpgradePolicy(ctx, DBUpgradePlatformDefault)
	if err != nil || !found {
		t.Fatalf("platform default missing: %v, %v", found, err)
	}
	if def.AutoUpgrade != DBAutoUpgradeOff || !def.BackupBefore || !def.VerifyAfter || !def.RevertOnFailure || def.WindowDuration != 2*time.Hour {
		t.Errorf("seeded default = %+v", def)
	}

	if _, found, _ := db.GetDBUpgradePolicy(ctx, "main"); found {
		t.Fatal("main has a policy before one was saved")
	}
	p := DBUpgradePolicy{
		DatabaseName: "main", AutoUpgrade: DBAutoUpgradePatch, WindowCron: "0 2 * * *", WindowDuration: time.Hour,
		WindowTimezone: "Europe/Berlin", VerifyAfter: true, RevertOnFailure: false, Notify: []string{"chn_1"},
		UpdatedBy: "admin", UpdatedAt: "2026-10-05T00:00:00Z",
	}
	if err := db.SaveDBUpgradePolicy(ctx, p); err != nil {
		t.Fatalf("SaveDBUpgradePolicy() error = %v", err)
	}
	got, found, err := db.GetDBUpgradePolicy(ctx, "main")
	if err != nil || !found {
		t.Fatalf("GetDBUpgradePolicy() = %v, %v", found, err)
	}
	if got.AutoUpgrade != DBAutoUpgradePatch || got.WindowTimezone != "Europe/Berlin" || got.RevertOnFailure || !got.BackupBefore || len(got.Notify) != 1 {
		t.Errorf("saved policy = %+v", got)
	}
	if list, _ := db.ListDBUpgradePolicies(ctx); len(list) != 2 {
		t.Errorf("ListDBUpgradePolicies() = %d, want 2", len(list))
	}
	if err := db.DeleteDBUpgradePolicy(ctx, "main"); err != nil {
		t.Fatalf("DeleteDBUpgradePolicy() error = %v", err)
	}
	if _, found, _ := db.GetDBUpgradePolicy(ctx, "main"); found {
		t.Error("policy still present after delete")
	}
	if err := db.DeleteDBUpgradePolicy(ctx, DBUpgradePlatformDefault); err == nil {
		t.Error("deleting the platform default succeeded")
	}
}
