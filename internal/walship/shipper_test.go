package walship

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var words = strings.Fields("build step pull image layer cached ready probe healthy cutover drain stop remove route cert renew start create attach volume network resolve warn error retry done")

func openPrimary(t testing.TB, path string) *sql.DB {
	t.Helper()
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS deploys (id INTEGER PRIMARY KEY AUTOINCREMENT, app TEXT NOT NULL, status TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, deploy_id INTEGER NOT NULL, kind TEXT NOT NULL, payload TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS events_deploy ON events(deploy_id)`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newRand(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic test data, not security
}

func logLine(r *rand.Rand, n int) string {
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(words[r.Intn(len(words))])
		b.WriteByte(' ')
	}
	return b.String()
}

// churn runs deploy-shaped writes: one insert, 12 status updates (own
// commits) and one 6-row event batch per deploy. It returns logical bytes written.
func churn(db *sql.DB, r *rand.Rand, deploys int, pace time.Duration, onCommit func(time.Time)) (int64, error) {
	var logical int64
	commit := func() {
		if onCommit != nil {
			onCommit(time.Now())
		}
		if pace > 0 {
			time.Sleep(pace)
		}
	}
	for i := 0; i < deploys; i++ {
		res, err := db.Exec(`INSERT INTO deploys(app,status,updated_at) VALUES(?,?,?)`, fmt.Sprintf("app-%d", i%40), "queued", time.Now().Format(time.RFC3339Nano))
		if err != nil {
			return logical, err
		}
		logical += 64
		commit()
		id, _ := res.LastInsertId()
		for s := 0; s < 12; s++ {
			if _, err := db.Exec(`UPDATE deploys SET status=?, updated_at=? WHERE id=?`, fmt.Sprintf("step-%d", s), time.Now().Format(time.RFC3339Nano), id); err != nil {
				return logical, err
			}
			logical += 40
			commit()
		}
		tx, err := db.Begin()
		if err != nil {
			return logical, err
		}
		for e := 0; e < 6; e++ {
			p := logLine(r, 512)
			if _, err := tx.Exec(`INSERT INTO events(deploy_id,kind,payload) VALUES(?,?,?)`, id, "log", p); err != nil {
				_ = tx.Rollback()
				return logical, err
			}
			logical += int64(len(p)) + 24
		}
		if err := tx.Commit(); err != nil {
			return logical, err
		}
		commit()
	}
	return logical, nil
}

func counts(t testing.TB, path string) (deploys, events int64, check string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.QueryRow(`SELECT count(*) FROM deploys`).Scan(&deploys); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil {
		t.Fatal(err)
	}
	return
}

func TestShipRestoreAcrossCheckpoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cp.db")
	db := openPrimary(t, path)
	defer func() { _ = db.Close() }()
	sink := DirSink{Dir: filepath.Join(dir, "replica")}
	sh, err := New(path, sink, 128<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sh.Close() }()
	ctx := context.Background()
	if err := sh.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	r := newRand(1)
	for round := 0; round < 12; round++ {
		if _, err := churn(db, r, 8, 0, nil); err != nil {
			t.Fatal(err)
		}
		if err := sh.Sync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	st := sh.Stats()
	if st.Checkpoints == 0 {
		t.Fatalf("expected at least one checkpoint, stats %+v", st)
	}
	dest := filepath.Join(dir, "restored.db")
	res, err := Restore(ctx, sink, dest)
	if err != nil {
		t.Fatal(err)
	}
	if res.Truncated {
		t.Fatalf("unexpected truncation: %+v", res)
	}
	pd, pe, _ := counts(t, path)
	rd, re, check := counts(t, dest)
	if check != "ok" || rd != pd || re != pe {
		t.Fatalf("restore mismatch: primary %d/%d replica %d/%d integrity %s (%+v, %+v)", pd, pe, rd, re, check, res, st)
	}
	if res.Epochs < 2 {
		t.Fatalf("expected the WAL to roll over, got %+v", res)
	}
}

func TestShipsFramesFromLiveWAL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.db")
	db := openPrimary(t, path)
	if _, err := churn(db, newRand(2), 2, 0, nil); err != nil {
		t.Fatal(err)
	}
	sh, err := New(path, DirSink{Dir: filepath.Join(dir, "r")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sh.Close() }()
	if err := sh.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sh.Stats().FrameBytes == 0 {
		t.Fatal("expected frames shipped from a live WAL")
	}
	_ = db.Close()
}
