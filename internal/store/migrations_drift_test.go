package store

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

const driftShortWindow = 40

var dropsMarkerRe = regexp.MustCompile(`(?m)^--\s*migrate:\s*drops\s+([a-z0-9_]+)\.([a-z0-9_*]+)`)

// driftBaseline freezes old migrations that dropped tables on purpose before
// the drops marker existed (0373 removed the legacy auth tables). Never add to it.
var driftBaseline = map[int]bool{373: true}

// schemaColumns maps table name to its column names.
type schemaColumns map[string][]string

func snapshotSchema(ctx context.Context, db *DB) (schemaColumns, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return nil, err
		}
		tables = append(tables, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()

	out := schemaColumns{}
	for _, tbl := range tables {
		cols, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT name FROM pragma_table_info('%s')`, tbl))
		if err != nil {
			return nil, err
		}
		for cols.Next() {
			var c string
			if err := cols.Scan(&c); err != nil {
				_ = cols.Close()
				return nil, err
			}
			out[tbl] = append(out[tbl], c)
		}
		if err := cols.Err(); err != nil {
			return nil, err
		}
		_ = cols.Close()
	}
	return out, nil
}

// droppedColumns lists table.column pairs present in before and missing in
// after that the migration SQL did not declare with a drops marker. A marker
// of the form "table.*" covers dropping the whole table.
func droppedColumns(before, after schemaColumns, sql string) []string {
	allowed := map[string]bool{}
	for _, m := range dropsMarkerRe.FindAllStringSubmatch(sql, -1) {
		allowed[m[1]+"."+m[2]] = true
	}
	var out []string
	for tbl, cols := range before {
		have := map[string]bool{}
		for _, c := range after[tbl] {
			have[c] = true
		}
		_, tableKept := after[tbl]
		for _, c := range cols {
			if have[c] || allowed[tbl+"."+c] || (!tableKept && allowed[tbl+".*"]) {
				continue
			}
			out = append(out, tbl+"."+c)
		}
	}
	sort.Strings(out)
	return out
}

func TestMigrationDrift_DetectsDroppedColumn(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "fixture.db"), WithMigrationCeiling(1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const fixture = `
CREATE TABLE drift_probe (id TEXT PRIMARY KEY, kept TEXT, lost TEXT);
`
	const rebuild = `
CREATE TABLE drift_probe_new (id TEXT PRIMARY KEY, kept TEXT);
INSERT INTO drift_probe_new (id, kept) SELECT id, kept FROM drift_probe;
DROP TABLE drift_probe;
ALTER TABLE drift_probe_new RENAME TO drift_probe;
`
	if _, err := db.ExecContext(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotSchema(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, rebuild); err != nil {
		t.Fatal(err)
	}
	after, err := snapshotSchema(ctx, db)
	if err != nil {
		t.Fatal(err)
	}

	if got := droppedColumns(before, after, rebuild); len(got) != 1 || got[0] != "drift_probe.lost" {
		t.Fatalf("unmarked drop must be caught, got %v", got)
	}
	if got := droppedColumns(before, after, "-- migrate: drops drift_probe.lost\n"+rebuild); len(got) != 0 {
		t.Fatalf("marked drop must pass, got %v", got)
	}
}

func TestMigrationDrift_TableDriven(t *testing.T) {
	before := schemaColumns{"a": {"x", "y"}, "b": {"z"}}
	cases := []struct {
		name  string
		after schemaColumns
		sql   string
		want  string
	}{
		{"added column only", schemaColumns{"a": {"x", "y", "w"}, "b": {"z"}}, "", ""},
		{"lost column", schemaColumns{"a": {"x"}, "b": {"z"}}, "", "a.y"},
		{"lost column marked", schemaColumns{"a": {"x"}, "b": {"z"}}, "-- migrate: drops a.y", ""},
		{"table dropped", schemaColumns{"a": {"x", "y"}}, "", "b.z"},
		{"table dropped marked", schemaColumns{"a": {"x", "y"}}, "-- migrate: drops b.*", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ""
			if d := droppedColumns(before, tc.after, tc.sql); len(d) > 0 {
				got = d[0]
			}
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// TestMigrationDrift_NoSilentColumnLoss applies the real chain one migration
// at a time. Short mode checks the newest migrations, the nightly lane all.
func TestMigrationDrift_NoSilentColumnLoss(t *testing.T) {
	ctx := context.Background()
	migs, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if testing.Short() && len(migs) > driftShortWindow {
		migs = migs[len(migs)-driftShortWindow:]
	}

	path := filepath.Join(t.TempDir(), "drift.db")
	prevCeiling := migs[0].version - 1
	open := func(ceiling int) *DB {
		db, err := Open(ctx, path, WithMigrationCeiling(ceiling))
		if err != nil {
			t.Fatalf("open at ceiling %d: %v", ceiling, err)
		}
		return db
	}
	var db *DB
	t.Cleanup(func() {
		if db != nil {
			_ = db.Close()
		}
	})
	before := schemaColumns{}
	if prevCeiling > 0 {
		db = open(prevCeiling)
		var err error
		if before, err = snapshotSchema(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range migs {
		if db != nil {
			_ = db.Close()
		}
		db = open(m.version)
		after, err := snapshotSchema(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if lost := droppedColumns(before, after, m.sql); len(lost) > 0 && !driftBaseline[m.version] {
			t.Errorf("migration %04d_%s silently dropped %v: carry them over in the rebuild or add %q", m.version, m.name, lost, "-- migrate: drops <table>.<column>")
		}
		before = after
	}
}
