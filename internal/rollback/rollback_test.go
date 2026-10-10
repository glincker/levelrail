package rollback

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/kit/upgrade"
)

func TestBuildPlanVerdicts(t *testing.T) {
	tests := []struct {
		name        string
		db, target  int
		want        upgrade.SchemaVerdict
		wantRestore bool
	}{
		{"equal schema", 100, 100, upgrade.VerdictBinaryOnly, false},
		{"older target needs restore", 100, 90, upgrade.VerdictRestoreRequired, true},
		{"newer target migrates forward", 100, 110, upgrade.VerdictForward, false},
		{"unknown target", 100, -1, upgrade.VerdictUnknown, false},
		{"unknown database", -1, 100, upgrade.VerdictUnknown, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := BuildPlan(PlanInput{CurrentVersion: "v2.0.0", CurrentSchemaVersion: tc.db, Target: Target{Version: "v1.0.0", SchemaVersion: tc.target, Retained: true}, ProgramName: "prog"})
			if p.Verdict != tc.want || p.RestoreRequired != tc.wantRestore {
				t.Fatalf("verdict=%s restore=%v, want %s %v", p.Verdict, p.RestoreRequired, tc.want, tc.wantRestore)
			}
		})
	}
}

func TestBuildPlanBackupCompatibility(t *testing.T) {
	now := time.Now().UTC()
	p := BuildPlan(PlanInput{
		CurrentVersion: "v2.0.0", CurrentSchemaVersion: 100, ProgramName: "prog",
		Target: Target{Version: "v1.0.0", SchemaVersion: 90},
		Backups: []BackupOption{
			{Name: "new.db", CreatedAt: now, SchemaVersion: 100},
			{Name: "old.db", CreatedAt: now.Add(-48 * time.Hour), SchemaVersion: 88},
		},
	})
	if p.LossSince == nil || !p.LossSince.Equal(now.Add(-48*time.Hour)) {
		t.Fatalf("data loss time = %v", p.LossSince)
	}
	if p.Backups[0].Compatible || !p.Backups[1].Compatible {
		t.Fatalf("compatibility = %+v", p.Backups)
	}
	if p.RestoreCommand == "" {
		t.Fatal("missing restore command")
	}
}

type fakeHost struct {
	dir        string
	bin        string
	bootOK     bool
	prevBootOK bool
	started    int
	stopped    int
	restored   string
	undone     string
	audits     []string
	running    string
}

func newFakeHost(t *testing.T) (*fakeHost, Deps) {
	t.Helper()
	dir := t.TempDir()
	h := &fakeHost{dir: dir, bin: filepath.Join(dir, "prog"), bootOK: true, prevBootOK: true}
	if err := os.WriteFile(h.bin, []byte("current-binary"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("target-binary"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	rel := Dir(h.bin)
	if _, err := Retain(rel, "prog", "v1.0.0", src, 90, time.Now()); err != nil {
		t.Fatal(err)
	}
	d := Deps{
		BinaryPath: h.bin, ReleasesDir: rel, BinaryName: "prog", Log: slog.New(slog.DiscardHandler),
		Backup: func(context.Context) (string, error) { return "pre.db", nil },
		Stop:   func(context.Context) error { h.stopped++; return nil },
		Start: func(context.Context) error {
			h.started++
			b, _ := os.ReadFile(h.bin)
			h.running = string(b)
			return nil
		},
		WaitHealthy: func(context.Context, time.Duration) error {
			if h.running == "target-binary" && !h.bootOK {
				return errors.New("exited with schema error")
			}
			if h.running == "current-binary" && !h.prevBootOK {
				return errors.New("previous also dead")
			}
			return nil
		},
		RestoreDB:     func(_ context.Context, name string) (string, error) { h.restored = name; return "kept.db", nil },
		UndoRestoreDB: func(_ context.Context, p string) error { h.undone = p; return nil },
		Audit:         func(_ context.Context, s string) error { h.audits = append(h.audits, s); return nil },
	}
	return h, d
}

func planFor(db, target int, backups ...BackupOption) Plan {
	return BuildPlan(PlanInput{CurrentVersion: "v2.0.0", CurrentSchemaVersion: db, ProgramName: "prog", Backups: backups,
		Target: Target{Version: "v1.0.0", SchemaVersion: target, Retained: true}})
}

func TestGuardRefusals(t *testing.T) {
	now := time.Now()
	good := BackupOption{Name: "good.db", CreatedAt: now, SchemaVersion: 85}
	bad := BackupOption{Name: "bad.db", CreatedAt: now, SchemaVersion: 99}
	tests := []struct {
		name string
		plan Plan
		opts Options
		want error
	}{
		{"wrong typed version", planFor(90, 90), Options{ConfirmVersion: "v9"}, ErrConfirmVersion},
		{"binary-only across newer schema refused", planFor(100, 90), Options{ConfirmVersion: "v1.0.0"}, ErrSchemaNewer},
		{"incompatible backup refused", planFor(100, 90, bad), Options{ConfirmVersion: "v1.0.0", RestoreBackup: "bad.db", ConfirmDataLoss: "bad.db"}, ErrBackupIncompatible},
		{"missing data loss confirmation", planFor(100, 90, good), Options{ConfirmVersion: "v1.0.0", RestoreBackup: "good.db"}, ErrConfirmDataLoss},
		{"unknown schema refused", planFor(100, -1), Options{ConfirmVersion: "v1.0.0"}, ErrSchemaUnknown},
		{"restore when not needed", planFor(90, 90, good), Options{ConfirmVersion: "v1.0.0", RestoreBackup: "good.db"}, ErrRestoreNotNeeded},
		{"binary-only equal schema ok", planFor(90, 90), Options{ConfirmVersion: "v1.0.0"}, nil},
		{"restore with both confirmations ok", planFor(100, 90, good), Options{ConfirmVersion: "v1.0.0", RestoreBackup: "good.db", ConfirmDataLoss: "good.db"}, nil},
		{"unknown accepted", planFor(100, -1), Options{ConfirmVersion: "v1.0.0", AcceptUnknown: true}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Guard(tc.plan, tc.opts)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestApplyRefusalChangesNothing(t *testing.T) {
	h, d := newFakeHost(t)
	_, err := Apply(context.Background(), d, planFor(100, 90), Options{ConfirmVersion: "v1.0.0", CurrentVersion: "v2.0.0"})
	if !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("err = %v", err)
	}
	if h.stopped != 0 || h.started != 0 {
		t.Fatalf("service touched: stop=%d start=%d", h.stopped, h.started)
	}
	if b, _ := os.ReadFile(h.bin); string(b) != "current-binary" {
		t.Fatalf("binary changed to %q", b)
	}
}

func TestApplyBinaryOnlySuccess(t *testing.T) {
	h, d := newFakeHost(t)
	res, err := Apply(context.Background(), d, planFor(90, 90), Options{ConfirmVersion: "v1.0.0", CurrentVersion: "v2.0.0", HealthTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeApplied || res.BackupName != "pre.db" {
		t.Fatalf("result = %+v", res)
	}
	if b, _ := os.ReadFile(h.bin); string(b) != "target-binary" {
		t.Fatalf("binary = %q", b)
	}
	if len(h.audits) != 2 || h.restored != "" {
		t.Fatalf("audits=%v restored=%q", h.audits, h.restored)
	}
}

func TestApplyHalfFailureRestoresPrevious(t *testing.T) {
	h, d := newFakeHost(t)
	h.bootOK = false
	res, err := Apply(context.Background(), d, planFor(90, 90), Options{ConfirmVersion: "v1.0.0", CurrentVersion: "v2.0.0", HealthTimeout: time.Second})
	if !errors.Is(err, ErrRecovered) {
		t.Fatalf("err = %v", err)
	}
	if res.Outcome != OutcomeRecovered {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if b, _ := os.ReadFile(h.bin); string(b) != "current-binary" {
		t.Fatalf("previous binary not restored: %q", b)
	}
	if h.running != "current-binary" || len(h.audits) != 2 {
		t.Fatalf("running=%q audits=%v", h.running, h.audits)
	}
}

func TestApplyRestoreFailureUndoesDatabase(t *testing.T) {
	h, d := newFakeHost(t)
	h.bootOK = false
	good := BackupOption{Name: "good.db", CreatedAt: time.Now(), SchemaVersion: 85}
	_, err := Apply(context.Background(), d, planFor(100, 90, good), Options{ConfirmVersion: "v1.0.0", CurrentVersion: "v2.0.0", RestoreBackup: "good.db", ConfirmDataLoss: "good.db", HealthTimeout: time.Second})
	if !errors.Is(err, ErrRecovered) {
		t.Fatalf("err = %v", err)
	}
	if h.restored != "good.db" || h.undone != "kept.db" {
		t.Fatalf("restored=%q undone=%q", h.restored, h.undone)
	}
}

func TestApplyRefusesTamperedRetainedBinary(t *testing.T) {
	h, d := newFakeHost(t)
	if err := os.WriteFile(filepath.Join(d.ReleasesDir, "prog-v1.0.0"), []byte("evil"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	_, err := Apply(context.Background(), d, planFor(90, 90), Options{ConfirmVersion: "v1.0.0", CurrentVersion: "v2.0.0"})
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v", err)
	}
	if h.stopped != 0 {
		t.Fatal("service stopped despite a bad checksum")
	}
}

func TestRetainPrunesToKeepCount(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	rel := filepath.Join(dir, "rel")
	base := time.Now()
	for i, v := range []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0"} {
		if _, err := Retain(rel, "prog", v, src, i, base); err != nil {
			t.Fatal(err)
		}
		past := base.Add(time.Duration(i-10) * time.Minute)
		_ = os.Chtimes(filepath.Join(rel, "prog-"+v), past, past)
	}
	list, err := List(rel, "prog")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != KeepCount {
		t.Fatalf("retained %d, want %d", len(list), KeepCount)
	}
	if _, err := Retain(rel, "prog", "../evil", src, 0, base); !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("path traversal not rejected: %v", err)
	}
}
