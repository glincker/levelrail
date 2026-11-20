package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxBackupLabelLen bounds a label so list output stays readable.
const MaxBackupLabelLen = 80

// NormalizeBackupLabel trims a label and refuses control characters or an over long value.
func NormalizeBackupLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > MaxBackupLabelLen {
		return "", fmt.Errorf("label must be at most %d characters", MaxBackupLabelLen)
	}
	for _, r := range label {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("label must not contain control characters")
		}
	}
	return label, nil
}

// SetBackupLabel stores the label for a backup attempt; an empty label removes it.
func (db *DB) SetBackupLabel(ctx context.Context, historyID, label string) error {
	if label == "" {
		if _, err := db.ExecContext(ctx, `DELETE FROM backup_labels WHERE history_id = ?`, historyID); err != nil {
			return fmt.Errorf("store: clear backup label %q: %w", historyID, err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_labels (history_id, label, created_at) VALUES (?, ?, ?)
		ON CONFLICT(history_id) DO UPDATE SET label = excluded.label
	`, historyID, label, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set backup label %q: %w", historyID, err)
	}
	return nil
}

// BackupLabels returns the labels for the given history ids; ids without one are absent.
func (db *DB) BackupLabels(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string)
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := db.reader().QueryContext(ctx, `SELECT history_id, label FROM backup_labels WHERE history_id IN (`+marks+`)`, args...) //nolint:gosec // only placeholders are interpolated
	if err != nil {
		return nil, fmt.Errorf("store: read backup labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, fmt.Errorf("store: scan backup label: %w", err)
		}
		out[id] = label
	}
	return out, rows.Err()
}
