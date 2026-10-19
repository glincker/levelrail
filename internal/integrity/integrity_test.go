package integrity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

const dbFile = "levelrail.db"

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func openDB(t *testing.T, dir string) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(dir, dbFile))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return db
}

// corruptFile overwrites the middle of a database file with garbage, in a temp dir only.
func corruptFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0) //nolint:gosec // temp dir path made by the test
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	junk := make([]byte, 4096)
	for i := range junk {
		junk[i] = 0xAB
	}
	for off := int64(4096); off+4096 <= info.Size(); off += 4096 {
		if _, err := f.WriteAt(junk, off); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.WriteAt(junk[:100], 0); err != nil {
		t.Fatal(err)
	}
}

func TestCheckerHealthyDatabase(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, dir)
	defer func() { _ = db.Close() }()
	c := NewChecker(db, quietLogger())
	for _, mode := range []string{ModeQuick, ModeFull} {
		if st := c.Check(context.Background(), mode); !st.Healthy || st.Mode != mode {
			t.Fatalf("mode %s: want healthy, got %+v", mode, st)
		}
	}
	if st := c.Status(); st.Checks != 2 || st.Failures != 0 || st.LastFullAt.IsZero() {
		t.Fatalf("unexpected counters: %+v", st)
	}
}

func TestBootCheckMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, dbFile)
	if err := BootCheck(context.Background(), path); err != nil {
		t.Fatalf("missing file must pass: %v", err)
	}
	db := openDB(t, dir)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := BootCheck(context.Background(), path); err != nil {
		t.Fatalf("healthy file must pass: %v", err)
	}
	corruptFile(t, path)
	if err := BootCheck(context.Background(), path); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("want ErrCorrupt, got %v", err)
	}
}

// TestRestoreDrill is the full drill: snapshot, corrupt the live file, detect it at boot,
// stage the latest verified snapshot, apply it as the next boot would, and read the data back.
func TestRestoreDrill(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := openDB(t, dir)
	if _, err := db.ExecContext(ctx, `CREATE TABLE drill_marker (v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO drill_marker (v) VALUES ('before-corruption')`); err != nil {
		t.Fatal(err)
	}
	mgr := cpbackup.NewManager(db, dir)
	if _, err := mgr.Create(ctx); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, dbFile)
	corruptFile(t, live)
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(live + suffix)
	}
	if err := BootCheck(ctx, live); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corruption not detected at boot: %v", err)
	}

	offline := cpbackup.NewManager(nil, dir)
	svc := NewService(nil, dir, offline, quietLogger())
	cands, err := Candidates(offline)
	if err != nil || len(cands) != 1 {
		t.Fatalf("want one candidate, got %v %v", cands, err)
	}
	pending, err := svc.Restore(ctx, "", "tester")
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if got, _ := ReadPending(dir); got == nil || got.Name != pending.Name {
		t.Fatalf("pending marker not written: %+v", got)
	}

	applied, err := ApplyPending(ctx, dir, dbFile, time.Now())
	if err != nil || applied == nil {
		t.Fatalf("apply: %v %v", applied, err)
	}
	if _, err := os.Stat(applied.Aside); err != nil {
		t.Fatalf("damaged file must be kept aside: %v", err)
	}
	if got, _ := ReadPending(dir); got != nil {
		t.Fatal("marker must be gone after apply")
	}
	if err := BootCheck(ctx, live); err != nil {
		t.Fatalf("restored database must pass: %v", err)
	}
	restored := openDB(t, dir)
	defer func() { _ = restored.Close() }()
	var v string
	if err := restored.QueryRowContext(ctx, `SELECT v FROM drill_marker`).Scan(&v); err != nil || v != "before-corruption" {
		t.Fatalf("data not restored: %q %v", v, err)
	}
}

func TestStageRejectsBadSnapshotsAndApplyDetectsTamper(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := openDB(t, dir)
	mgr := cpbackup.NewManager(db, dir)
	info, err := mgr.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	if _, err := Stage(ctx, dir, "../../etc/passwd", "x", time.Now()); !errors.Is(err, cpbackup.ErrInvalidName) {
		t.Fatalf("want invalid name, got %v", err)
	}
	if _, err := Stage(ctx, dir, "missing.db", "x", time.Now()); err == nil {
		t.Fatal("a snapshot that does not exist must be rejected")
	}
	if _, err := Stage(ctx, dir, info.Name, "x", time.Now()); err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(dir, cpbackup.DirName, info.Name)
	if err := os.WriteFile(snap, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPending(ctx, dir, dbFile, time.Now()); err == nil {
		t.Fatal("a snapshot changed after staging must not be applied")
	}
	if got, _ := ReadPending(dir); got != nil {
		t.Fatal("marker must be cleared after a failed apply so boots cannot loop")
	}
}

func TestLatestVerifiedSkipsDamagedSnapshots(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db := openDB(t, dir)
	mgr := cpbackup.NewManager(db, dir)
	good, err := mgr.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	bad, err := mgr.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if err := os.WriteFile(filepath.Join(dir, cpbackup.DirName, bad.Name), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LatestVerified(ctx, cpbackup.NewManager(nil, dir))
	if err != nil || got != good.Name {
		t.Fatalf("want %s (older, intact), got %q %v", good.Name, got, err)
	}
	if _, err := LatestVerified(ctx, cpbackup.NewManager(nil, t.TempDir())); !errors.Is(err, ErrNoVerifiedBackup) {
		t.Fatalf("want ErrNoVerifiedBackup, got %v", err)
	}
}
