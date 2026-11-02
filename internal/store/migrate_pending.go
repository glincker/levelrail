package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

// PendingMigration names one migration for plans and dry-run reports.
type PendingMigration struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

// MigrationSpec is one forward-only migration a caller supplies to
// MigrateFileWith, so a dry run can be tested with a deliberately bad one.
type MigrationSpec struct {
	Version int
	Name    string
	SQL     string
}

// PendingMigrationList lists the embedded migrations with from < version <= to.
// to <= 0 means no upper bound.
func PendingMigrationList(from, to int) ([]PendingMigration, error) {
	all, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	out := []PendingMigration{}
	for _, m := range all {
		if m.version > from && (to <= 0 || m.version <= to) {
			out = append(out, PendingMigration{Version: m.version, Name: m.name})
		}
	}
	return out, nil
}

// SnapshotFile copies the database at src to dest with VACUUM INTO over a
// read-only connection, so the live file is never written. dest must not exist.
func SnapshotFile(ctx context.Context, src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("snapshot destination %s already exists", dest)
	}
	q := url.Values{}
	q.Add("mode", "ro")
	q.Add("_pragma", "busy_timeout(5000)")
	sdb, err := sql.Open("sqlite", "file:"+src+"?"+q.Encode())
	if err != nil {
		return fmt.Errorf("open %s read-only: %w", src, err)
	}
	defer func() { _ = sdb.Close() }()
	sdb.SetMaxOpenConns(1)
	if _, err := sdb.ExecContext(ctx, "VACUUM INTO "+quoteSQLString(dest)); err != nil { //nolint:gosec // VACUUM INTO takes no bind parameter; the path is quoted
		_ = os.Remove(dest)
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return nil
}

// MigrateFile applies this binary's embedded pending migrations to the
// database file at path and returns what ran. Callers use it on a copy only.
func MigrateFile(ctx context.Context, path string) ([]PendingMigration, error) {
	all, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	specs := make([]MigrationSpec, 0, len(all))
	for _, m := range all {
		specs = append(specs, MigrationSpec{Version: m.version, Name: m.name, SQL: m.sql})
	}
	return MigrateFileWith(ctx, path, specs)
}

// MigrateFileWith applies the specs not yet recorded in path's
// schema_migrations, in version order, each in its own transaction, then runs
// integrity_check. It stops at the first failing migration.
func MigrateFileWith(ctx context.Context, path string, specs []MigrationSpec) ([]PendingMigration, error) {
	sqlDB, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{DB: sqlDB}
	defer func() { _ = sqlDB.Close() }()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')))`); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, err := db.appliedVersions(ctx)
	if err != nil {
		return nil, err
	}
	ms := make([]migration, 0, len(specs))
	for _, s := range specs {
		ms = append(ms, migration{version: s.Version, name: s.Name, sql: s.SQL})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	ran := []PendingMigration{}
	for _, m := range pendingMigrations(ms, applied) {
		if err := db.applyMigration(ctx, m); err != nil {
			return ran, fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
		ran = append(ran, PendingMigration{Version: m.version, Name: m.name})
	}
	var verdict string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&verdict); err != nil {
		return ran, fmt.Errorf("integrity check: %w", err)
	}
	if !strings.EqualFold(verdict, "ok") {
		return ran, fmt.Errorf("integrity check failed: %s", verdict)
	}
	return ran, nil
}
