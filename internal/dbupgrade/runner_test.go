package dbupgrade

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	fakeFrom   = "16.9"
	fakeTarget = "16.11"
)

type runnerFixture struct {
	store    *fakeStore
	rt       *fakeRuntime
	runner   *Runner
	notifier *fakeNotifier
}

func newRunnerFixture(t *testing.T, engine string) *runnerFixture {
	t.Helper()
	s := newFakeStore(store.DesiredDatabase{Name: "main", Engine: engine, Version: fakeFrom, BackupTargetID: "tgt"})
	rt := newFakeRuntime(s, "main")
	cat, err := parseCatalog([]byte(testCatalog))
	if err != nil {
		t.Fatal(err)
	}
	if engine == store.EngineMariaDB {
		cat.engines[engine] = EngineCatalog{Levels: []string{KindMajor, KindMajor, KindPatch}, AutoMax: KindPatch, ImageRevert: false}
	}
	n := &fakeNotifier{}
	return &runnerFixture{store: s, rt: rt, notifier: n, runner: &Runner{
		Store: s, Runtime: rt, Catalog: cat, Notifier: n, HealthTimeout: 40 * time.Millisecond, Poll: time.Millisecond,
	}}
}

func (fx *runnerFixture) createRun(t *testing.T, revert bool) {
	t.Helper()
	d := fx.store.dbs["main"]
	run := store.DBUpgradeRun{
		ID: "dbu_test", DatabaseName: "main", Engine: d.Engine, FromVersion: fakeFrom, ToVersion: fakeTarget, Kind: KindPatch,
		Source: store.DBUpgradeSourceManual, State: store.DBUpgradeStatePending, VerifyAfter: true, RevertOnFailure: revert,
		Notify: []string{"chn_1"}, CreatedAt: "2026-10-10T00:00:00Z",
	}
	if err := fx.store.CreateDBUpgradeRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}

func (fx *runnerFixture) final(t *testing.T) store.DBUpgradeRun {
	t.Helper()
	run, err := fx.store.GetDBUpgradeRun(context.Background(), "dbu_test")
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRunnerStateMachine(t *testing.T) {
	tests := []struct {
		name        string
		engine      string
		noRevert    bool
		setup       func(fx *runnerFixture)
		wantState   string
		wantPath    string
		wantVersion string
		wantReason  string
		wantNoBump  bool
	}{
		{name: "happy path", wantState: store.DBUpgradeStateSucceeded, wantVersion: fakeTarget},
		{name: "another job owns the database", setup: func(fx *runnerFixture) { fx.rt.busy = "a restore of this database is running" },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "restore", wantNoBump: true},
		{name: "backup fails", setup: func(fx *runnerFixture) { fx.rt.fail["RunBackup"] = true },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "backup failed", wantNoBump: true},
		{name: "backup recorded as failed", setup: func(fx *runnerFixture) { fx.rt.backupStatus = store.BackupStatusFailed },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "backup", wantNoBump: true},
		{name: "verification errors", setup: func(fx *runnerFixture) { fx.rt.fail["RunVerify"] = true },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "verifying", wantNoBump: true},
		{name: "verification does not pass", setup: func(fx *runnerFixture) { fx.rt.verifyStatus = store.BackupVerificationStatusFailed },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "failed verification", wantNoBump: true},
		{name: "target image missing", setup: func(fx *runnerFixture) { fx.rt.fail["CheckImage"] = true },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "image is not available", wantNoBump: true},
		{name: "snapshot fails", setup: func(fx *runnerFixture) { fx.rt.fail["SnapshotData"] = true },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "snapshot", wantNoBump: true},
		{name: "bump fails, image revert", setup: func(fx *runnerFixture) { fx.rt.fail["SetVersion:"+fakeTarget] = true },
			wantState: store.DBUpgradeStateReverted, wantPath: RevertPathImage, wantVersion: fakeFrom, wantReason: "applying version"},
		{name: "unhealthy, image revert", setup: func(fx *runnerFixture) { fx.rt.unhealthy[fakeTarget] = true },
			wantState: store.DBUpgradeStateReverted, wantPath: RevertPathImage, wantVersion: fakeFrom, wantReason: "health check"},
		{name: "data incompatible, snapshot revert", setup: func(fx *runnerFixture) {
			fx.rt.unhealthy[fakeTarget] = true
			fx.rt.writesNewFormat = true
		}, wantState: store.DBUpgradeStateReverted, wantPath: RevertPathSnapshot, wantVersion: fakeFrom, wantReason: "image revert did not come up"},
		{name: "snapshot restore fails, backup restore", setup: func(fx *runnerFixture) {
			fx.rt.unhealthy[fakeTarget] = true
			fx.rt.writesNewFormat = true
			fx.rt.fail["RestoreSnapshot"] = true
		}, wantState: store.DBUpgradeStateReverted, wantPath: RevertPathBackup, wantVersion: fakeFrom, wantReason: "volume_snapshot revert failed"},
		{name: "every revert fails", setup: func(fx *runnerFixture) {
			fx.rt.unhealthy[fakeTarget] = true
			fx.rt.writesNewFormat = true
			fx.rt.fail["RestoreSnapshot"] = true
			fx.rt.fail["RestoreBackup"] = true
		}, wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "REVERT FAILED"},
		{name: "revert off leaves the new version", noRevert: true, setup: func(fx *runnerFixture) { fx.rt.unhealthy[fakeTarget] = true },
			wantState: store.DBUpgradeStateFailed, wantVersion: fakeTarget, wantReason: "revert_on_failure is off"},
		{name: "engine without image revert goes to the snapshot", engine: store.EngineMariaDB, setup: func(fx *runnerFixture) { fx.rt.unhealthy[fakeTarget] = true },
			wantState: store.DBUpgradeStateReverted, wantPath: RevertPathSnapshot, wantVersion: fakeFrom},
		{name: "no backup target", setup: func(fx *runnerFixture) {
			d := fx.store.dbs["main"]
			d.BackupTargetID = ""
			fx.store.dbs["main"] = d
		}, wantState: store.DBUpgradeStateFailed, wantVersion: fakeFrom, wantReason: "no backup target", wantNoBump: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := tt.engine
			if engine == "" {
				engine = store.EnginePostgres
			}
			fx := newRunnerFixture(t, engine)
			if tt.setup != nil {
				tt.setup(fx)
			}
			fx.createRun(t, !tt.noRevert)
			if err := fx.runner.Drive(context.Background(), "dbu_test"); err != nil {
				t.Fatalf("Drive() error = %v", err)
			}
			got := fx.final(t)
			if got.State != tt.wantState || got.RevertPath != tt.wantPath {
				t.Fatalf("state %q path %q (reason %q), want %q %q", got.State, got.RevertPath, got.Reason, tt.wantState, tt.wantPath)
			}
			if v := fx.store.version("main"); v != tt.wantVersion {
				t.Errorf("database version = %q, want %q", v, tt.wantVersion)
			}
			if !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", got.Reason, tt.wantReason)
			}
			if tt.wantNoBump && fx.rt.called("SetVersion:"+fakeTarget) > 0 {
				t.Error("the version was bumped although the run failed before the bump")
			}
			if fx.rt.suspended {
				t.Error("the database was left stopped")
			}
			if got.FinishedAt == "" || got.Timings[got.State] == "" {
				t.Errorf("finished run missing timings: %+v", got)
			}
			if len(fx.notifier.sent) == 0 {
				t.Error("no notification was sent")
			}
		})
	}
}

func TestRunnerResumesAfterRestartAtEveryStep(t *testing.T) {
	steps := []string{"Busy", "RunBackup", "RunVerify", "CheckImage", "ImageDigest", "SnapshotData", "SetVersion:" + fakeTarget, "Health"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			fx := newRunnerFixture(t, store.EnginePostgres)
			fx.createRun(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			fx.rt.crashAt, fx.rt.cancel = step, cancel
			if err := fx.runner.Drive(ctx, "dbu_test"); err == nil {
				t.Fatal("Drive() returned nil although the process was stopped mid-step")
			}
			if mid := fx.final(t); store.DBUpgradeTerminal(mid.State) {
				t.Fatalf("a stop at %s finished the run as %q (%s)", step, mid.State, mid.Reason)
			}

			restarted := *fx.runner
			if err := restarted.Drive(context.Background(), "dbu_test"); err != nil {
				t.Fatalf("resumed Drive() error = %v", err)
			}
			got := fx.final(t)
			if got.State != store.DBUpgradeStateSucceeded {
				t.Fatalf("after restart at %s: state %q reason %q", step, got.State, got.Reason)
			}
			if v := fx.store.version("main"); v != fakeTarget {
				t.Errorf("version = %q, want %q", v, fakeTarget)
			}
		})
	}
}

func TestRunnerRestartDuringBackupTakesAFreshBackup(t *testing.T) {
	fx := newRunnerFixture(t, store.EnginePostgres)
	fx.createRun(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	fx.rt.crashAt, fx.rt.cancel = "RunBackup", cancel
	_ = fx.runner.Drive(ctx, "dbu_test")
	first := fx.final(t).BackupID
	if err := fx.runner.Drive(context.Background(), "dbu_test"); err != nil {
		t.Fatal(err)
	}
	got := fx.final(t)
	if got.BackupID == first || fx.rt.called("RunBackup") != 2 {
		t.Errorf("backup id %q (first %q), RunBackup calls %d: want a second, fresh backup", got.BackupID, first, fx.rt.called("RunBackup"))
	}
}

func TestRunnerIsIdempotentOnFinishedRuns(t *testing.T) {
	fx := newRunnerFixture(t, store.EnginePostgres)
	fx.createRun(t, true)
	if err := fx.runner.Drive(context.Background(), "dbu_test"); err != nil {
		t.Fatal(err)
	}
	calls := len(fx.rt.calls)
	if err := fx.runner.Drive(context.Background(), "dbu_test"); err != nil {
		t.Fatal(err)
	}
	if len(fx.rt.calls) != calls {
		t.Errorf("driving a finished run made %d more runtime calls", len(fx.rt.calls)-calls)
	}
}

func TestServerMatches(t *testing.T) {
	tests := []struct {
		server, expected string
		want             bool
	}{
		{"16.11 (Debian 16.11-1.pgdg120+1)", "16.11", true},
		{"16.10", "16.11", false},
		{"16.11", "16", true},
		{"11.4.8-MariaDB-ubu2404", "11.4.8", true},
		{"v1.27.1", "v1.27.1", true},
		{"24.8.14.39", "24.8", true},
		{"garbage", "16.11", true},
	}
	for _, tt := range tests {
		if got := serverMatches(tt.server, tt.expected); got != tt.want {
			t.Errorf("serverMatches(%q, %q) = %v, want %v", tt.server, tt.expected, got, tt.want)
		}
	}
}
