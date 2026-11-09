package store

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestStatementTimingRedaction(t *testing.T) {
	cases := []struct{ in, want string }{
		{`SELECT v FROM t WHERE k = 'fake-value'`, `SELECT v FROM t WHERE k = ?`},
		{"SELECT v -- don't\nFROM t WHERE k = 'fake-a'", `SELECT v FROM t WHERE k = ?`},
		{`SELECT /* it's */ v FROM t WHERE k = 'fake-b'`, `SELECT v FROM t WHERE k = ?`},
		{`SELECT v FROM t WHERE k = "fake-c" AND n = 4242`, `SELECT v FROM t WHERE k = ? AND n = ?`},
		{`SELECT v FROM t2 WHERE b = X'deadbeef' AND f = 1.5`, `SELECT v FROM t2 WHERE b = ? AND f = ?`},
		{`SELECT v FROM t WHERE k = 'it''s fake' AND id = ?1`, `SELECT v FROM t WHERE k = ? AND id = ?1`},
	}
	for _, c := range cases {
		if got := redactStatement(c.in); got != c.want {
			t.Errorf("redact(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	got := redactStatement("SELECT " + strings.Repeat("é", 100))
	if len(got) > maxLoggedStatement || !utf8.ValidString(got) {
		t.Fatalf("cut split a rune: %d bytes, valid %v", len(got), utf8.ValidString(got))
	}
}

func TestStatementTimingCommitAndRollback(t *testing.T) {
	db := openTestDB(t)
	withTiming(t, time.Nanosecond, 20)
	SetStatementLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE timing_tx (v TEXT)`); err != nil {
		t.Fatal(err)
	}
	ResetSlowStatements()
	tctx, stats := WithStatementStats(ctx)
	tx, err := db.BeginTx(tctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(tctx, `INSERT INTO timing_tx (v) VALUES (?)`, "x"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("rollback after commit = %v, want sql.ErrTxDone", err)
	}
	tx2, err := db.BeginTx(tctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, _ := stats.Snapshot(); got != 3 {
		t.Fatalf("statements = %d, want insert, COMMIT and ROLLBACK", got)
	}
	seen := map[string]bool{}
	for _, s := range SlowStatements() {
		seen[s.SQL] = true
	}
	if !seen["COMMIT"] || !seen["ROLLBACK"] {
		t.Fatalf("ring = %v", seen)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM timing_tx`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("commit lost: n=%d err=%v", n, err)
	}
}

func TestStatementTimingConcurrentRecording(t *testing.T) {
	db := openTestDB(t)
	withTiming(t, time.Nanosecond, 5)
	SetStatementLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, stats := WithStatementStats(context.Background())
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 25 {
				var n int
				if err := db.reader().QueryRowContext(ctx, `SELECT ?`, g*100+i).Scan(&n); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got, _ := stats.Snapshot(); got != 200 {
		t.Fatalf("statements = %d, want 200", got)
	}
	if len(SlowStatements()) > 5 {
		t.Fatal("ring exceeded its bound")
	}
}
