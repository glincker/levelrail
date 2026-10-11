package selfupgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeHost struct {
	t        *testing.T
	dir      string
	bin      string
	liveDB   string
	dbSchema int

	running    bool
	stops      int
	starts     int
	healthy    func() error
	restored   int
	newBinary  string
	badSum     bool
	sigErr     error
	probe      ProbeInfo
	migrateErr error
	migrateTo  int
	backupErr  error
	// startMigrates makes the new binary bump the live schema on start, as
	// a real release applying its migrations would.
	startMigrates int
	panicOnStop   bool
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	dir := t.TempDir()
	h := &fakeHost{t: t, dir: dir, bin: filepath.Join(dir, "levelrail"), liveDB: filepath.Join(dir, "levelrail.db"),
		dbSchema: 10, running: true, newBinary: "NEW", probe: ProbeInfo{Version: "v0.5.0", Schema: 11}, migrateTo: 11}
	h.write(h.bin, "OLD")
	h.write(h.liveDB, "db-v10")
	return h
}

func (h *fakeHost) write(path, content string) {
	h.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *fakeHost) read(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // test temp path
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

func (h *fakeHost) deps() Deps {
	return Deps{
		BinaryPath: h.bin, WorkDir: filepath.Join(h.dir, "work"), Asset: "levelrail-linux-amd64",
		Journal: NewJournal(h.dir), Now: time.Now,
		Fetch: func(_ context.Context, _, asset, dir string) (Fetched, error) {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return Fetched{}, err
			}
			f := Fetched{BinaryPath: filepath.Join(dir, asset), ChecksumsPath: filepath.Join(dir, "checksums.txt"), BundlePath: filepath.Join(dir, "b.json")}
			h.write(f.BinaryPath, h.newBinary)
			sum := sha256.Sum256([]byte(h.newBinary))
			digest := hex.EncodeToString(sum[:])
			if h.badSum {
				digest = strings.Repeat("0", 64)
			}
			h.write(f.ChecksumsPath, digest+"  "+asset+"\n")
			return f, nil
		},
		VerifySig: func(context.Context, string, string) (string, error) { return "ok", h.sigErr },
		Probe:     func(context.Context, string) (ProbeInfo, error) { return h.probe, nil },
		DBSchema:  func(context.Context) (int, error) { return h.dbSchema, nil },
		Backup: func(context.Context) (BackupInfo, error) {
			if h.backupErr != nil {
				return BackupInfo{}, h.backupErr
			}
			snap := filepath.Join(h.dir, "snap.db")
			h.write(snap, h.read(h.liveDB))
			return BackupInfo{Name: "snap.db", SnapshotPath: snap}, nil
		},
		MigrateTest: func(context.Context, string, string) (int, error) { return h.migrateTo, h.migrateErr },
		Stop: func(context.Context) error {
			if h.panicOnStop {
				panic("host command killed")
			}
			h.stops++
			h.running = false
			return nil
		},
		Start: func(context.Context) error {
			h.starts++
			h.running = true
			if h.read(h.bin) == "NEW" && h.startMigrates > 0 {
				h.dbSchema = h.startMigrates
				h.write(h.liveDB, "db-v11-by-new")
			}
			return nil
		},
		WaitHealthy: func(context.Context, time.Duration) error {
			if h.read(h.bin) == "NEW" && h.healthy != nil {
				return h.healthy()
			}
			return nil
		},
		RestoreDB: func(_ context.Context, snap string) (string, error) {
			h.restored++
			kept := h.liveDB + ".kept"
			h.write(kept, h.read(h.liveDB))
			h.write(h.liveDB, h.read(snap))
			h.dbSchema = 10
			return kept, nil
		},
	}
}

func req() Request {
	return Request{ID: "att-1", CurrentVersion: "v0.4.0", TargetVersion: "v0.5.0", Initiator: "tester"}
}

func TestApplyOutcomes(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name        string
		setup       func(h *fakeHost, r *Request)
		wantOutcome string
		wantErr     error
		wantStep    string
		wantBinary  string
		wantDB      string
		wantStops   int
	}{
		{name: "success", wantOutcome: OutcomeSucceeded, wantBinary: "NEW", wantDB: "db-v10", wantStops: 1},
		{name: "checksum mismatch", setup: func(h *fakeHost, _ *Request) { h.badSum = true },
			wantOutcome: OutcomeRefused, wantErr: ErrChecksumMismatch, wantStep: StepChecksum, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "bad signature", setup: func(h *fakeHost, _ *Request) { h.sigErr = fmt.Errorf("%w: nope", ErrSignatureInvalid) },
			wantOutcome: OutcomeRefused, wantErr: ErrSignatureInvalid, wantStep: StepSignature, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "breaking change not acknowledged", setup: func(_ *fakeHost, r *Request) {
			r.Breaking = []Breaking{{ID: "ports", RequiresAck: true}}
		}, wantOutcome: OutcomeRefused, wantErr: ErrBreakingNotAcked, wantStep: StepGuard, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "breaking change acknowledged", setup: func(_ *fakeHost, r *Request) {
			r.Breaking = []Breaking{{ID: "ports", RequiresAck: true}}
			r.Acked = []string{"ports"}
		}, wantOutcome: OutcomeSucceeded, wantBinary: "NEW", wantDB: "db-v10", wantStops: 1},
		{name: "target older than running", setup: func(_ *fakeHost, r *Request) { r.TargetVersion = "v0.3.0" },
			wantOutcome: OutcomeRefused, wantErr: ErrDowngrade, wantStep: StepGuard, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "binary schema older than database", setup: func(h *fakeHost, _ *Request) { h.probe.Schema = 9 },
			wantOutcome: OutcomeRefused, wantErr: ErrDowngrade, wantStep: StepProbe, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "binary reports another version", setup: func(h *fakeHost, _ *Request) { h.probe.Version = "v0.9.9" },
			wantOutcome: OutcomeRefused, wantErr: ErrVersionMismatch, wantStep: StepProbe, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "backup fails", setup: func(h *fakeHost, _ *Request) { h.backupErr = boom },
			wantOutcome: OutcomeFailed, wantErr: boom, wantStep: StepBackup, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "migration dry run fails", setup: func(h *fakeHost, _ *Request) { h.migrateErr = boom },
			wantOutcome: OutcomeFailed, wantErr: ErrMigrationCheck, wantStep: StepMigrationCheck, wantBinary: "OLD", wantDB: "db-v10"},
		{name: "unhealthy after swap with schema migrated", setup: func(h *fakeHost, _ *Request) {
			h.startMigrates = 11
			h.healthy = func() error { return boom }
		}, wantOutcome: OutcomeRolledBack, wantErr: ErrNotHealthy, wantStep: StepHealth, wantBinary: "OLD", wantDB: "db-v10", wantStops: 2},
		{name: "unhealthy after swap without schema change", setup: func(h *fakeHost, _ *Request) {
			h.healthy = func() error { return boom }
		}, wantOutcome: OutcomeRolledBack, wantErr: ErrNotHealthy, wantStep: StepHealth, wantBinary: "OLD", wantDB: "db-v10", wantStops: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFakeHost(t)
			r := req()
			if tt.setup != nil {
				tt.setup(h, &r)
			}
			a, err := Apply(context.Background(), h.deps(), r)
			if a.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %s, want %s (err %v)", a.Outcome, tt.wantOutcome, err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if a.FailedStep != tt.wantStep {
				t.Fatalf("failed step = %q, want %q", a.FailedStep, tt.wantStep)
			}
			if got := h.read(h.bin); got != tt.wantBinary {
				t.Fatalf("binary = %s, want %s", got, tt.wantBinary)
			}
			if got := h.read(h.liveDB); got != tt.wantDB {
				t.Fatalf("db = %s, want %s", got, tt.wantDB)
			}
			if h.stops != tt.wantStops {
				t.Fatalf("stops = %d, want %d", h.stops, tt.wantStops)
			}
			if !h.running {
				t.Fatalf("service left stopped")
			}
			if _, statErr := os.Stat(h.bin + ".prev"); statErr == nil && a.Outcome != OutcomeFailed {
				t.Fatalf("previous binary copy left behind after %s", a.Outcome)
			}
		})
	}
}

func TestApplyRollbackKeepsDataWhenSchemaUnchanged(t *testing.T) {
	h := newFakeHost(t)
	h.healthy = func() error { return errors.New("down") }
	if _, err := Apply(context.Background(), h.deps(), req()); err == nil {
		t.Fatal("want error")
	}
	if h.restored != 0 {
		t.Fatalf("database restored %d times although the schema did not change", h.restored)
	}
}

func TestApplyRestoresSnapshotWhenSchemaMigrated(t *testing.T) {
	h := newFakeHost(t)
	h.startMigrates = 11
	h.healthy = func() error { return errors.New("down") }
	a, _ := Apply(context.Background(), h.deps(), req())
	if h.restored != 1 || a.Outcome != OutcomeRolledBack {
		t.Fatalf("restored=%d outcome=%s", h.restored, a.Outcome)
	}
	if h.read(h.liveDB+".kept") != "db-v11-by-new" {
		t.Fatal("failed run's database was not kept for inspection")
	}
}

func TestApplyRollbackItselfFails(t *testing.T) {
	h := newFakeHost(t)
	h.healthy = func() error { return errors.New("down") }
	d := h.deps()
	d.WaitHealthy = func(context.Context, time.Duration) error { return errors.New("never healthy") }
	a, err := Apply(context.Background(), d, req())
	if a.Outcome != OutcomeFailed || err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("outcome=%s err=%v", a.Outcome, err)
	}
	if !strings.Contains(err.Error(), ".prev") {
		t.Fatalf("error does not name the previous binary path: %v", err)
	}
}

func TestRecoverAfterRestartInTheMiddle(t *testing.T) {
	t.Run("killed while stopping, before the swap", func(t *testing.T) {
		h := newFakeHost(t)
		h.panicOnStop = true
		d := h.deps()
		func() {
			defer func() { _ = recover() }()
			_, _ = Apply(context.Background(), d, req())
		}()
		running, err := d.Journal.Running()
		if err != nil || len(running) != 1 {
			t.Fatalf("running journal = %v, %v", running, err)
		}
		h.panicOnStop = false
		h.running = false
		a, rerr := Recover(context.Background(), d, running[0], time.Second)
		if a.Outcome != OutcomeFailed || rerr == nil {
			t.Fatalf("outcome=%s err=%v", a.Outcome, rerr)
		}
		if h.read(h.bin) != "OLD" || !h.running {
			t.Fatalf("binary=%s running=%v", h.read(h.bin), h.running)
		}
	})
	t.Run("killed after the swap, new release never confirmed", func(t *testing.T) {
		h := newFakeHost(t)
		h.startMigrates = 11
		d := h.deps()
		d.Start = func(context.Context) error {
			h.running = true
			h.dbSchema = 11
			h.write(h.liveDB, "db-v11-by-new")
			panic("host command killed")
		}
		func() {
			defer func() { _ = recover() }()
			_, _ = Apply(context.Background(), d, req())
		}()
		running, _ := d.Journal.Running()
		if len(running) != 1 || h.read(h.bin) != "NEW" {
			t.Fatalf("journal=%d binary=%s", len(running), h.read(h.bin))
		}
		a, err := Recover(context.Background(), h.deps(), running[0], time.Second)
		if a.Outcome != OutcomeRolledBack {
			t.Fatalf("outcome=%s err=%v", a.Outcome, err)
		}
		if h.read(h.bin) != "OLD" || h.read(h.liveDB) != "db-v10" {
			t.Fatalf("binary=%s db=%s", h.read(h.bin), h.read(h.liveDB))
		}
	})
	t.Run("killed after the health check passed", func(t *testing.T) {
		h := newFakeHost(t)
		d := h.deps()
		a := Attempt{ID: "att-9", Outcome: OutcomeRunning, StartedAt: time.Now(),
			Steps: []StepRecord{{Name: StepStop, Status: StatusOK}, {Name: StepSwap, Status: StatusOK}, {Name: StepHealth, Status: StatusOK}}}
		got, err := Recover(context.Background(), d, a, time.Second)
		if err != nil || got.Outcome != OutcomeSucceeded {
			t.Fatalf("outcome=%s err=%v", got.Outcome, err)
		}
	})
}

func TestJournalRoundTrip(t *testing.T) {
	j := NewJournal(t.TempDir())
	a := Attempt{ID: "att-1", Outcome: OutcomeRunning, StartedAt: time.Now(), Steps: []StepRecord{{Name: StepGuard, Status: StatusOK}}}
	if err := j.Save(a); err != nil {
		t.Fatal(err)
	}
	if err := j.Save(Attempt{ID: "../escape"}); err == nil {
		t.Fatal("unsafe id accepted")
	}
	list, err := j.List()
	if err != nil || len(list) != 1 || list[0].ID != "att-1" {
		t.Fatalf("list = %+v, %v", list, err)
	}
}
