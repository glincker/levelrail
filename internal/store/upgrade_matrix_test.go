package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// seedEveryTable inserts one synthetic row into each table, using only the
// columns SQLite requires, and returns the tables that accepted a row.
func seedEveryTable(ctx context.Context, t *testing.T, db *DB) []string {
	t.Helper()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable fk: %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, n)
	}
	_ = rows.Close()

	var seeded []string
	for _, tbl := range tables {
		cols, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%q)`, tbl))
		if err != nil {
			t.Fatalf("table_info %s: %v", tbl, err)
		}
		var names, marks []string
		var args []any
		for cols.Next() {
			var cid, notnull, pk int
			var name, typ string
			var dflt any
			if err := cols.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				t.Fatalf("scan column: %v", err)
			}
			if notnull == 0 && pk == 0 || dflt != nil {
				continue
			}
			names = append(names, fmt.Sprintf("%q", name))
			marks = append(marks, "?")
			switch up := strings.ToUpper(typ); {
			case strings.Contains(up, "INT"), strings.Contains(up, "REAL"), strings.Contains(up, "NUM"):
				args = append(args, 1)
			case strings.Contains(up, "BLOB"):
				args = append(args, []byte("seed"))
			default:
				args = append(args, "seed")
			}
		}
		_ = cols.Close()
		q := fmt.Sprintf(`INSERT INTO %q DEFAULT VALUES`, tbl)
		if len(names) > 0 {
			q = fmt.Sprintf(`INSERT INTO %q (%s) VALUES (%s)`, tbl, strings.Join(names, ","), strings.Join(marks, ","))
		}
		if _, err := db.ExecContext(ctx, q, args...); err == nil {
			seeded = append(seeded, tbl)
		}
	}
	return seeded
}

func tableRows(ctx context.Context, db *DB, table string) (int, bool) {
	var n int
	if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table)).Scan(&n); err != nil {
		return 0, false
	}
	return n, true
}

func TestUpgradeMatrix_SeededDataSurvivesUpgradeToHead(t *testing.T) {
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	var checkpoints []int
	step := len(all) / 8
	if step < 1 {
		step = 1
	}
	for i := step; i < len(all)-1; i += step {
		checkpoints = append(checkpoints, all[i].version)
	}
	head := all[len(all)-1].version

	for _, from := range checkpoints {
		t.Run(fmt.Sprintf("from_%04d", from), func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "state.db")
			old, err := Open(ctx, path, WithMigrationCeiling(from))
			if err != nil {
				t.Fatalf("open at %d: %v", from, err)
			}
			seeded := seedEveryTable(ctx, t, old)
			if len(seeded) == 0 {
				t.Fatalf("no table accepted a seed row at version %d", from)
			}
			if err := old.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}

			cur, err := Open(ctx, path)
			if err != nil {
				t.Fatalf("upgrade %d to %d: %v", from, head, err)
			}
			defer func() { _ = cur.Close() }()

			for _, tbl := range seeded {
				n, exists := tableRows(ctx, cur, tbl)
				if exists && n == 0 {
					t.Errorf("table %s lost its seeded row upgrading %d to %d", tbl, from, head)
				}
			}
			if err := cur.MigrationsCurrent(ctx); err != nil {
				t.Errorf("after upgrade: %v", err)
			}
		})
	}
}

func TestUpgradeMatrix_OlderBinaryRefusesNewerSchema(t *testing.T) {
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	head, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open head: %v", err)
	}
	if err := head.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	for _, ceiling := range []int{all[0].version, all[len(all)/2].version, all[len(all)-2].version} {
		if _, err := Open(ctx, path, WithMigrationCeiling(ceiling)); !errors.Is(err, ErrSchemaNewer) {
			t.Errorf("ceiling %d: err = %v, want ErrSchemaNewer", ceiling, err)
		}
	}
}
