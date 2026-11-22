package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrBackupPinned is returned when a delete targets a backup pinned to keep forever.
var ErrBackupPinned = errors.New("store: backup is pinned")

// SetBackupPinned pins or unpins one backup. It reports ErrBackupHistoryNotFound for an unknown id.
func (db *DB) SetBackupPinned(ctx context.Context, historyID string, pinned bool) error {
	if !pinned {
		if _, err := db.ExecContext(ctx, `DELETE FROM backup_pins WHERE history_id = ?`, historyID); err != nil {
			return fmt.Errorf("store: unpin backup %q: %w", historyID, err)
		}
		return nil
	}
	if _, err := db.GetBackupHistory(ctx, historyID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_pins (history_id, created_at) VALUES (?, ?)
		ON CONFLICT(history_id) DO NOTHING
	`, historyID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: pin backup %q: %w", historyID, err)
	}
	return nil
}

// PinnedBackups returns the subset of ids that are pinned.
func (db *DB) PinnedBackups(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := db.reader().QueryContext(ctx, `SELECT history_id FROM backup_pins WHERE history_id IN (`+marks+`)`, args...) //nolint:gosec // only placeholders are interpolated
	if err != nil {
		return nil, fmt.Errorf("store: read backup pins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan backup pin: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}
