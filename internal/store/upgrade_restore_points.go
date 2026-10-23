package store

import (
	"context"
	"database/sql"
	"fmt"
)

// CountOnlineNodes reads how many nodes are marked online from the database
// file, read-only and without migrating it, so a host command of any release can ask.
func CountOnlineNodes(ctx context.Context, path string) (int, error) {
	sdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = sdb.Close() }()
	var n int
	if err := sdb.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE status = 'online'`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count online nodes: %w", err)
	}
	return n, nil
}

// UpgradeRestorePoint is one kept pre-upgrade restore point (migrations/2606).
type UpgradeRestorePoint struct {
	ID            string
	Version       string
	SchemaVersion int
	AttemptID     string
	SizeBytes     int64
	HasBinary     bool
	HasUnit       bool
	CreatedAt     string
}

// ReplaceUpgradeRestorePoints makes the table mirror pts, the host's restore point directory.
func (db *DB) ReplaceUpgradeRestorePoints(ctx context.Context, pts []UpgradeRestorePoint) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restore point sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM upgrade_restore_points`); err != nil {
		return fmt.Errorf("clear restore points: %w", err)
	}
	for _, p := range pts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO upgrade_restore_points
			(id, version, schema_version, attempt_id, size_bytes, has_binary, has_unit, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.Version, p.SchemaVersion, p.AttemptID, p.SizeBytes, p.HasBinary, p.HasUnit, p.CreatedAt); err != nil {
			return fmt.Errorf("store restore point %s: %w", p.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore point sync: %w", err)
	}
	return nil
}

// ListUpgradeRestorePoints returns the restore points, newest first.
func (db *DB) ListUpgradeRestorePoints(ctx context.Context) ([]UpgradeRestorePoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, version, schema_version, attempt_id, size_bytes, has_binary, has_unit, created_at
		FROM upgrade_restore_points ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list restore points: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []UpgradeRestorePoint
	for rows.Next() {
		var p UpgradeRestorePoint
		if err := rows.Scan(&p.ID, &p.Version, &p.SchemaVersion, &p.AttemptID, &p.SizeBytes, &p.HasBinary, &p.HasUnit, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan restore point: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
