package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func benchDB(b *testing.B, t Tuning) *DB {
	b.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if err := db.ApplyTuning(ctx, t); err != nil {
		b.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE bench_rows (id INTEGER PRIMARY KEY, grp INTEGER, body TEXT)`); err != nil {
		b.Fatal(err)
	}
	return db
}

func benchModes() map[string]Tuning {
	return map[string]Tuning{
		"sync_full":    {Synchronous: "full"},
		"sync_normal":  {Synchronous: "normal"},
		"normal_tuned": {Synchronous: "normal", CacheSizeMB: 32, MmapSizeMB: 256},
	}
}

// BenchmarkWriteHeavy measures one autocommit insert per iteration, the reconcile write pattern.
func BenchmarkWriteHeavy(b *testing.B) {
	for name, t := range benchModes() {
		b.Run(name, func(b *testing.B) {
			db := benchDB(b, t)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := db.ExecContext(ctx, `INSERT INTO bench_rows (grp, body) VALUES (?, ?)`, i%50, "payload-payload-payload"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkListHeavy measures a filtered, ordered page read from a populated table.
func BenchmarkListHeavy(b *testing.B) {
	for name, t := range benchModes() {
		b.Run(name, func(b *testing.B) {
			db := benchDB(b, t)
			ctx := context.Background()
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				b.Fatal(err)
			}
			for i := 0; i < 20000; i++ {
				if _, err := tx.ExecContext(ctx, `INSERT INTO bench_rows (grp, body) VALUES (?, ?)`, i%50, fmt.Sprintf("row-%d", i)); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			r := db.reader()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rows, err := r.QueryContext(ctx, `SELECT id, body FROM bench_rows WHERE grp = ? ORDER BY id DESC LIMIT 100`, i%50)
				if err != nil {
					b.Fatal(err)
				}
				for rows.Next() {
					var id int
					var body string
					if err := rows.Scan(&id, &body); err != nil {
						b.Fatal(err)
					}
				}
				_ = rows.Close()
			}
		})
	}
}
