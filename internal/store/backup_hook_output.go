package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SaveBackupHookOutput stores the already masked output of a failing hook.
func (db *DB) SaveBackupHookOutput(ctx context.Context, historyID, phase, output string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_hook_outputs (backup_history_id, phase, output, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (backup_history_id) DO UPDATE SET phase = excluded.phase, output = excluded.output, created_at = excluded.created_at
	`, historyID, phase, output, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save backup hook output %q: %w", historyID, err)
	}
	return nil
}

// GetBackupHookOutput returns the stored hook output, or "" when there is none.
func (db *DB) GetBackupHookOutput(ctx context.Context, historyID string) (phase, output string, err error) {
	err = db.reader().QueryRowContext(ctx, `SELECT phase, output FROM backup_hook_outputs WHERE backup_history_id = ?`, historyID).Scan(&phase, &output)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("store: get backup hook output %q: %w", historyID, err)
	}
	return phase, output, nil
}
