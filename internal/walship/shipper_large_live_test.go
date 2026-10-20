package walship

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveLargeDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("live WAL shipping benchmark")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "p.db")
	db := openPrimary(t, path)
	r := newRand(3)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 220000; i++ {
		if _, err := tx.Exec(`INSERT INTO events(deploy_id,kind,payload) VALUES(?,?,?)`, i/6, "log", logLine(r, 512)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	sh, err := New(path, DirSink{Dir: filepath.Join(dir, "replica")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	if err := sh.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := time.Since(t0)
	base := sh.Stats()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = sh.Run(ctx, 100*time.Millisecond) }()
	start := time.Now()
	logical, err := churn(db, newRand(8), 300, 2*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	dur := time.Since(start)
	cancel()
	<-done
	st := sh.Stats()
	_ = sh.Close()
	_ = db.Close()
	fi, _ := os.Stat(path)
	gz := gzipSize(t, path)
	model := gz * int64(dur/(5*time.Second)+1)
	t.Logf("db=%.1fMB gz=%.1fMB firstSnapshot=%s (%.1fMB shipped) | churn dur=%s logical=%.2fMB seg shipped=%.2fMB (%.2fMB per logical MB) | 5s-snapshot model=%.1fMB (%.1fMB per logical MB)",
		float64(fi.Size())/(1<<20), float64(gz)/(1<<20), first.Round(time.Millisecond), float64(base.SnapshotBytes)/(1<<20),
		dur.Round(time.Millisecond), float64(logical)/(1<<20), float64(st.SegmentBytes)/(1<<20), float64(st.SegmentBytes)/float64(logical),
		float64(model)/(1<<20), float64(model)/float64(logical))
}
