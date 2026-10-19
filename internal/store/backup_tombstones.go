package store

import (
	"context"
	"fmt"
	"time"
)

// BackupTombstone is a stored backup object that still has to be deleted from its target.
type BackupTombstone struct {
	ID        int64
	TargetID  string
	ObjectKey string
	Reason    string
	Attempts  int
	CreatedAt string
}

// AddBackupTombstone records an object whose delete failed; repeats for the same object are ignored.
func (db *DB) AddBackupTombstone(ctx context.Context, targetID, objectKey, reason string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO backup_object_tombstones (target_id, object_key, reason, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(target_id, object_key) DO UPDATE SET reason = excluded.reason`,
		targetID, objectKey, reason, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: add backup tombstone %q: %w", objectKey, err)
	}
	return nil
}

// ListBackupTombstones returns up to limit pending tombstones, oldest first.
func (db *DB) ListBackupTombstones(ctx context.Context, limit int) ([]BackupTombstone, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, target_id, object_key, reason, attempts, created_at FROM backup_object_tombstones ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list backup tombstones: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BackupTombstone
	for rows.Next() {
		var t BackupTombstone
		if err := rows.Scan(&t.ID, &t.TargetID, &t.ObjectKey, &t.Reason, &t.Attempts, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan backup tombstone: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list backup tombstones: %w", err)
	}
	return out, nil
}

// CountBackupTombstones returns how many objects are still waiting to be deleted.
func (db *DB) CountBackupTombstones(ctx context.Context) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM backup_object_tombstones`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count backup tombstones: %w", err)
	}
	return n, nil
}

// ResolveBackupTombstone removes a tombstone once its object is gone.
func (db *DB) ResolveBackupTombstone(ctx context.Context, id int64) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM backup_object_tombstones WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: resolve backup tombstone %d: %w", id, err)
	}
	return nil
}

// BumpBackupTombstone records a failed retry.
func (db *DB) BumpBackupTombstone(ctx context.Context, id int64, reason string) error {
	if _, err := db.ExecContext(ctx, `UPDATE backup_object_tombstones SET attempts = attempts + 1, reason = ? WHERE id = ?`, reason, id); err != nil {
		return fmt.Errorf("store: bump backup tombstone %d: %w", id, err)
	}
	return nil
}
