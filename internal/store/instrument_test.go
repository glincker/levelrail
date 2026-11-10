package store

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTiming turns timing on for one test and puts the process default back afterwards.
func withTiming(t *testing.T, slow time.Duration, ring int) {
	t.Helper()
	ResetSlowStatements()
	SetStatementTiming(StatementTimingConfig{Enabled: true, RingSize: ring})
	timing.slowNS.Store(int64(slow))
	t.Cleanup(func() {
		SetStatementTiming(StatementTimingConfig{Enabled: false, SlowMS: 100, RingSize: 20})
		SetStatementLogger(nil)
		ResetSlowStatements()
	})
}

func TestStatementTimingCountsEveryPath(t *testing.T) {
	db := openTestDB(t)
	withTiming(t, time.Hour, 20)
	base := context.Background()
	if _, err := db.ExecContext(base, `CREATE TABLE timing_probe (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatal(err)
	}
	ctx, stats := WithStatementStats(base)

	if _, err := db.ExecContext(ctx, `INSERT INTO timing_probe (v) VALUES (?)`, "a"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryContext(ctx, `SELECT v FROM timing_probe`)
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM timing_probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := db.reader().QueryRowContext(ctx, `SELECT count(*) FROM timing_probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO timing_probe (v) VALUES (?)`, "b"); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM timing_probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.PrepareContext(ctx, `SELECT v FROM timing_probe WHERE id = ?`)
	if err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 2; id++ {
		var v string
		if err := stmt.QueryRowContext(ctx, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got, total := stats.Snapshot(); got != 9 || total <= 0 {
		t.Fatalf("statements = %d (total %v), want 9 (8 statements plus COMMIT) with a positive total", got, total)
	}
	if n != 2 {
		t.Fatalf("tx semantics changed: count = %d, want 2", n)
	}

	SetStatementTiming(StatementTimingConfig{Enabled: false})
	if err := db.QueryRowContext(ctx, `SELECT 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if got, _ := stats.Snapshot(); got != 9 {
		t.Fatalf("off path counted: %d", got)
	}
}

func TestStatementTimingKeepsErrorsAndCancellation(t *testing.T) {
	db := openTestDB(t)
	withTiming(t, time.Hour, 20)
	_, err := db.ExecContext(context.Background(), `SELECT * FROM no_such_table`)
	if err == nil || !strings.Contains(err.Error(), "no such table") {
		t.Fatalf("error changed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.QueryContext(ctx, `SELECT 1`); err == nil {
		t.Fatal("cancelled context ran the statement")
	}
}

func TestStatementTimingSlowLogHasNoValues(t *testing.T) {
	db := openTestDB(t)
	withTiming(t, time.Nanosecond, 20)
	var buf bytes.Buffer
	SetStatementLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE timing_secret (v TEXT, w TEXT)`); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	ResetSlowStatements()
	if _, err := db.ExecContext(ctx, `INSERT INTO timing_secret (v, w) VALUES (?, 'inline-literal-value')`, "bound-value-xyz"); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "INSERT INTO timing_secret") != 1 {
		t.Fatalf("want exactly one slow line, got %q", out)
	}
	if strings.Contains(out, "bound-value-xyz") || strings.Contains(out, "inline-literal-value") {
		t.Fatalf("log leaked a value: %q", out)
	}
	if !strings.Contains(out, "INSERT INTO timing_secret (v, w) VALUES (?, ?)") {
		t.Fatalf("log lost the statement text: %q", out)
	}
	ring := SlowStatements()
	if len(ring) != 1 || strings.Contains(ring[0].SQL, "inline-literal-value") {
		t.Fatalf("ring = %+v", ring)
	}
}

func TestStatementTimingRingIsBounded(t *testing.T) {
	withTiming(t, time.Hour, 3)
	for i, ms := range []float64{5, 1, 9, 3, 7} {
		timing.remember("SELECT "+string(rune('a'+i)), ms)
	}
	timing.remember("SELECT c", 2)
	got := SlowStatements()
	if len(got) != 3 || got[0].MaxMS != 9 || got[0].Count != 2 || got[2].MaxMS != 5 {
		t.Fatalf("ring = %+v", got)
	}
	SetStatementTiming(StatementTimingConfig{Enabled: true, RingSize: 1})
	if got := SlowStatements(); len(got) != 1 || got[0].MaxMS != 9 {
		t.Fatalf("live shrink = %+v", got)
	}
}

func openTimingBenchDB(b *testing.B, driverName string) *sql.DB {
	b.Helper()
	path := filepath.Join(b.TempDir(), "bench.db")
	db, err := sql.Open(driverName, "file:"+path)
	if err != nil {
		b.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT); INSERT INTO t (v) VALUES ('x')`); err != nil {
		b.Fatal(err)
	}
	return db
}

func benchSelect(ctx context.Context, b *testing.B, db *sql.DB) {
	b.Helper()
	b.ReportAllocs()
	var v string
	for b.Loop() {
		if err := db.QueryRowContext(ctx, `SELECT v FROM t WHERE id = ?`, 1).Scan(&v); err != nil {
			b.Fatal(err)
		}
	}
}

// Compare allocs/op and ns/op of Raw against Off (wrapper present, setting off) and On.
func BenchmarkStatementTiming(b *testing.B) {
	b.Cleanup(func() { SetStatementTiming(StatementTimingConfig{Enabled: false}) })
	b.Run("Raw", func(b *testing.B) {
		SetStatementTiming(StatementTimingConfig{Enabled: false})
		benchSelect(context.Background(), b, openTimingBenchDB(b, "sqlite"))
	})
	b.Run("Off", func(b *testing.B) {
		SetStatementTiming(StatementTimingConfig{Enabled: false})
		benchSelect(context.Background(), b, openTimingBenchDB(b, timedDriverName))
	})
	b.Run("On", func(b *testing.B) {
		SetStatementTiming(StatementTimingConfig{Enabled: true, SlowMS: 60000})
		ctx, _ := WithStatementStats(context.Background())
		benchSelect(ctx, b, openTimingBenchDB(b, timedDriverName))
	})
}
