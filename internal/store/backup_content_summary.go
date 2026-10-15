package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SetBackupContentSummary records the stable object counts captured while
// backupHistoryID's dump was taken.
func (db *DB) SetBackupContentSummary(ctx context.Context, backupHistoryID, summary string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_content_summaries (backup_history_id, summary, recorded_at)
		VALUES (?, ?, ?)
		ON CONFLICT(backup_history_id) DO UPDATE SET summary = excluded.summary, recorded_at = excluded.recorded_at
	`, backupHistoryID, summary, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set backup content summary %q: %w", backupHistoryID, err)
	}
	return nil
}

// GetBackupContentSummary returns the recorded summary, or "" when none
// was recorded (older backups, or counts that moved during the dump).
func (db *DB) GetBackupContentSummary(ctx context.Context, backupHistoryID string) (string, error) {
	var summary string
	err := db.QueryRowContext(ctx, `SELECT summary FROM backup_content_summaries WHERE backup_history_id = ?`, backupHistoryID).Scan(&summary)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get backup content summary %q: %w", backupHistoryID, err)
	}
	return summary, nil
}
