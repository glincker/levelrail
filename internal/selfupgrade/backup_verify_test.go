package selfupgrade

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/rollback"
	"github.com/GLINCKER/levelrail/internal/store"
)

type verifyFixture struct {
	dir, live, snapshot, key string
}

func newVerifyFixture(t *testing.T) verifyFixture {
	t.Helper()
	dir := t.TempDir()
	f := verifyFixture{dir: dir, live: filepath.Join(dir, "levelrail.db"), snapshot: filepath.Join(dir, "snap.db"), key: filepath.Join(dir, "master.key")}
	db, err := sql.Open("sqlite", "file:"+f.live)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE apps (id TEXT PRIMARY KEY, name TEXT)`,
		`CREATE TABLE users (id TEXT PRIMARY KEY)`,
		`CREATE TABLE nodes (id TEXT PRIMARY KEY)`,
		`CREATE TABLE runtime_settings (k TEXT PRIMARY KEY, v TEXT)`,
		`CREATE TABLE schema_migrations (version INTEGER)`,
		`INSERT INTO schema_migrations VALUES (1)`,
		`INSERT INTO apps VALUES ('a1','one'),('a2','two')`,
		`INSERT INTO users VALUES ('u1')`,
		`INSERT INTO nodes VALUES ('n1'),('n2'),('n3')`,
	}
	for i := 0; i < 400; i++ {
		stmts = append(stmts, `INSERT INTO runtime_settings VALUES ('k`+strings.Repeat("x", i%7)+string(rune('a'+i%26))+string(rune('a'+i/26))+`','`+strings.Repeat("v", 200)+`')`)
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if _, err := db.Exec(`VACUUM INTO ?`, f.snapshot); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	_, sum, err := store.FileSHA256(f.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.snapshot+".sha256", []byte(sum), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.key, []byte("not-a-real-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f verifyFixture) opts(free int64) BackupVerifyOptions {
	return BackupVerifyOptions{
		Snapshot: f.snapshot, ScratchBase: f.dir,
		Restore:   func(scratch, snap string) (string, error) { return rollback.SwapInBackup(scratch, snap, time.Now()) },
		FreeBytes: func(string) (int64, error) { return free, nil },
		KeyFiles:  []string{f.key},
	}
}

func dirSums(t *testing.T, f verifyFixture) (string, string) {
	t.Helper()
	_, a, err := store.FileSHA256(f.live)
	if err != nil {
		t.Fatal(err)
	}
	_, b, err := store.FileSHA256(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, ".verify-backup-*"))
	return m
}

func TestBackupVerifyGoodBackupPasses(t *testing.T) {
	f := newVerifyFixture(t)
	liveBefore, keyBefore := dirSums(t, f)
	note, err := VerifyBackup(context.Background(), f.opts(1<<40))
	if err != nil {
		t.Fatalf("good backup refused: %v", err)
	}
	for _, want := range []string{"apps=2", "users=1", "nodes=3", "runtime_settings=400", "master.key present"} {
		if !strings.Contains(note, want) {
			t.Fatalf("report %q missing %q", note, want)
		}
	}
	liveAfter, keyAfter := dirSums(t, f)
	if liveBefore != liveAfter || keyBefore != keyAfter {
		t.Fatal("the live data changed during the check")
	}
	if len(leftovers(t, f.dir)) != 0 {
		t.Fatal("scratch directory not removed")
	}
}

func TestBackupVerifyCorruptedBackupRefused(t *testing.T) {
	f := newVerifyFixture(t)
	raw, err := os.ReadFile(f.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0x01
	if err := os.WriteFile(f.snapshot, raw, 0o600); err != nil { //nolint:gosec // test temp path
		t.Fatal(err)
	}
	liveBefore, keyBefore := dirSums(t, f)
	_, err = VerifyBackup(context.Background(), f.opts(1<<40))
	if !errors.Is(err, ErrBackupVerify) {
		t.Fatalf("flipped bit not refused: %v", err)
	}
	liveAfter, keyAfter := dirSums(t, f)
	if liveBefore != liveAfter || keyBefore != keyAfter || len(leftovers(t, f.dir)) != 0 {
		t.Fatal("refusal touched the live data or left a scratch copy")
	}
}

func TestBackupVerifyCorruptedHeaderRefusedWithoutSidecar(t *testing.T) {
	f := newVerifyFixture(t)
	_ = os.Remove(f.snapshot + ".sha256")
	raw, _ := os.ReadFile(f.snapshot)
	for i := 0; i < 64; i++ {
		raw[i] ^= 0xff
	}
	if err := os.WriteFile(f.snapshot, raw, 0o600); err != nil { //nolint:gosec // test temp path
		t.Fatal(err)
	}
	if _, err := VerifyBackup(context.Background(), f.opts(1<<40)); !errors.Is(err, ErrBackupVerify) {
		t.Fatalf("damaged database not refused: %v", err)
	}
}

func TestBackupVerifyShortDiskSkipsWithWarning(t *testing.T) {
	f := newVerifyFixture(t)
	note, err := VerifyBackup(context.Background(), f.opts(1))
	if err != nil {
		t.Fatalf("short disk must skip, not fail: %v", err)
	}
	if !strings.Contains(note, "skipped") || !strings.Contains(note, "NOT proven") {
		t.Fatalf("warning missing: %q", note)
	}
	if len(leftovers(t, f.dir)) != 0 {
		t.Fatal("a skipped check made a scratch copy")
	}
}

func TestPreUpgradeBackupVerifyGate(t *testing.T) {
	bad := errors.New("restore broke")
	h := newFakeHost(t)
	d := h.deps()
	d.VerifyBackup = func(context.Context, string) (string, error) { return "", bad }
	a, err := Apply(context.Background(), d, req())
	if a.Outcome != OutcomeFailed || !errors.Is(err, bad) || !strings.Contains(err.Error(), "backup-now") || h.stops != 0 {
		t.Fatalf("outcome=%s err=%v stops=%d, want a refusal naming backup-now", a.Outcome, err, h.stops)
	}

	h = newFakeHost(t)
	d = h.deps()
	d.SkipBackup = true
	d.VerifyBackup = func(context.Context, string) (string, error) { return "", bad }
	if a, err = Apply(context.Background(), d, req()); err != nil || a.Outcome != OutcomeSucceeded {
		t.Fatalf("skip-backup outcome=%s err=%v", a.Outcome, err)
	}

	h = newFakeHost(t)
	d = h.deps()
	d.VerifyBackup = func(context.Context, string) (string, error) { return "rows match", nil }
	if a, err = Apply(context.Background(), d, req()); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range a.Steps {
		found = found || (s.Name == StepBackup && strings.Contains(s.Detail, "rows match"))
	}
	if !found {
		t.Fatal("backup step detail lacks the verification result")
	}
}
