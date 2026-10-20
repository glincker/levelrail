package dbupgrade

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// liveBackups records succeeded backup and verification rows without S3:
// this test proves the in-place bump and health checks, not the upload.
type liveBackups struct{ db *store.DB }

func (l liveBackups) RunBackup(ctx context.Context, id, name, _, _, target string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := l.db.StartBackupHistory(ctx, store.BackupHistory{ID: id, DatabaseName: name, TargetID: target, ObjectKey: "live/" + id, StartedAt: now}); err != nil {
		return err
	}
	return l.db.FinishBackupHistory(ctx, id, store.BackupStatusSucceeded, 1, "sum", "", now)
}

func (l liveBackups) VerifyBackup(ctx context.Context, id, backupID, _, by string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := l.db.StartBackupVerification(ctx, store.BackupVerification{ID: id, BackupHistoryID: backupID, CheckedBy: by, StartedAt: now}); err != nil {
		return err
	}
	return l.db.FinishBackupVerification(ctx, id, store.BackupVerificationStatusPassed, true, true, true, 1, "", now)
}

func TestRunnerPostgresPatchInPlace_Live(t *testing.T) {
	dockertest.SkipIfShort(t)
	rt, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := rt.InspectByName(pingCtx, "levelrail-ping"); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	const name = "levelrail-upgrade-live"
	const from, to = "16.10-alpine", "16.11-alpine"
	ctx := context.Background()
	container, volume := database.ContainerName(name), dataVolume(name)
	cleanup := func() {
		if st, err := rt.InspectByName(ctx, container); err == nil && st != nil {
			_ = rt.Remove(ctx, st.ID, true)
		}
		_ = rt.RemoveVolume(ctx, volume)
	}
	cleanup()
	t.Cleanup(cleanup)

	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.SaveBackupTarget(ctx, store.BackupTarget{ID: "tgt", Name: "live", Provider: "aws", Region: "us-east-1", Bucket: "b", CreatedAt: "2026-10-10T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: from}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDatabaseBackupSchedule(ctx, name, "tgt", "", 0, 0); err != nil {
		t.Fatal(err)
	}

	ctrl := database.New(name, db, rt, database.WithPostgresCredentials(&database.PostgresCredentials{Username: name, Password: "live-secret"}))
	reconcileCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		for reconcileCtx.Err() == nil {
			_, _ = ctrl.Reconcile(reconcileCtx)
			_ = sleepCtx(reconcileCtx, 500*time.Millisecond)
		}
	}()

	cat, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	lb := liveBackups{db: db}
	runner := &Runner{
		Store: db, Catalog: cat, HealthTimeout: 3 * time.Minute, Poll: time.Second,
		Runtime: &DockerRuntime{Store: db, Docker: rt, Backups: lb, Verifier: lb, CopyVolume: backup.CopyVolume, WipeVolume: backup.WipeVolume, StopWait: 2 * time.Minute},
	}
	m := &Manager{Store: db, Runner: runner, Advisor: Advisor{Catalog: cat, EOLWarn: DefaultEOLWarn}}

	if ok, _, err := runner.waitHealthy(ctx, &store.DBUpgradeRun{DatabaseName: name, PhaseStartedAt: time.Now().UTC().Format(time.RFC3339)}, from, true); err != nil || !ok {
		t.Fatalf("database never came up on %s: %v", from, err)
	}

	missing, err := m.UpgradeNow(ctx, name, "16.99-alpine", "live")
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Drive(ctx, missing.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetDBUpgradeRun(ctx, missing.ID); got.State != store.DBUpgradeStateFailed || !strings.Contains(got.Reason, "not available") {
		t.Fatalf("missing tag run = %s %q, want failed before any change", got.State, got.Reason)
	}

	run, err := m.UpgradeNow(ctx, name, to, "live")
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Drive(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetDBUpgradeRun(ctx, run.ID)
	if got.State != store.DBUpgradeStateSucceeded {
		t.Fatalf("run = %s %q", got.State, got.Reason)
	}
	if got.FromImageDigest == "" || got.SnapshotVolume == "" {
		t.Errorf("run did not record the pre-upgrade image and snapshot: %+v", got)
	}
	cmd, _ := HealthCommand(store.EnginePostgres)
	rc, err := rt.Exec(ctx, container, cmd)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(rc)
	_ = rc.Close()
	if v := parseHealthVersion(string(out)); !strings.HasPrefix(v, "16.11") {
		t.Errorf("server reports %q after the upgrade, want 16.11", v)
	}
}
