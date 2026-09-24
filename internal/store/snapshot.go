package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrSchemaNewer is returned when a database's schema version is newer than
// this binary's newest migration.
var ErrSchemaNewer = errors.New("database schema is newer than this binary supports")

// MaxSchemaVersion is the newest migration version this binary knows.
func MaxSchemaVersion() (int, error) {
	migrations, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, nil
	}
	return migrations[len(migrations)-1].version, nil
}

// SnapshotTo writes a consistent copy of the live database to dest using
// VACUUM INTO, verifies it with integrity_check, and returns its size and
// sha256. dest must not exist; it only appears once the copy is verified.
func (db *DB) SnapshotTo(ctx context.Context, dest string) (size int64, sum string, err error) {
	tmp := dest + ".tmp"
	_ = os.Remove(tmp)
	if _, statErr := os.Stat(dest); statErr == nil {
		return 0, "", fmt.Errorf("snapshot destination %s already exists", dest)
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO "+quoteSQLString(tmp)); err != nil {
		_ = os.Remove(tmp)
		return 0, "", fmt.Errorf("vacuum into snapshot: %w", err)
	}
	if _, err := InspectSnapshot(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return 0, "", fmt.Errorf("finalize snapshot: %w", err)
	}
	return FileSHA256(dest)
}

// FileSHA256 returns the size and hex sha256 of the file at path.
func FileSHA256(path string) (int64, string, error) {
	f, err := os.Open(path) //nolint:gosec // path is built by the caller from a validated name
	if err != nil {
		return 0, "", fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", fmt.Errorf("hash %s: %w", path, err)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// InspectSnapshot opens the SQLite file read-only, runs integrity_check, and
// returns its schema migration version.
func InspectSnapshot(ctx context.Context, path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	sdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = sdb.Close() }()

	var result string
	if err := sdb.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return 0, fmt.Errorf("integrity check %s: %w", path, err)
	}
	if result != "ok" {
		return 0, fmt.Errorf("integrity check %s failed: %s", path, result)
	}
	var version int
	if err := sdb.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, fmt.Errorf("%s is not a control plane database: %w", path, err)
	}
	return version, nil
}

func quoteSQLString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
