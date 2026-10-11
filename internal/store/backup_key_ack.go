package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BackupKeyAck records that the operator stored the encryption private key
// off this server for the key whose public half is Recipient.
type BackupKeyAck struct {
	Recipient      string
	AcknowledgedAt time.Time
	AcknowledgedBy string
}

// GetBackupKeyAck returns the acknowledgement, and false when none exists.
func (db *DB) GetBackupKeyAck(ctx context.Context) (BackupKeyAck, bool, error) {
	var (
		a  BackupKeyAck
		at string
	)
	err := db.QueryRowContext(ctx, `SELECT recipient, acknowledged_at, acknowledged_by FROM backup_key_ack WHERE id = 1`).
		Scan(&a.Recipient, &at, &a.AcknowledgedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupKeyAck{}, false, nil
	}
	if err != nil {
		return BackupKeyAck{}, false, fmt.Errorf("store: get backup key acknowledgement: %w", err)
	}
	a.AcknowledgedAt, _ = time.Parse(time.RFC3339Nano, at)
	return a, true, nil
}

// SetBackupKeyAck stores the acknowledgement, replacing any earlier one.
func (db *DB) SetBackupKeyAck(ctx context.Context, recipient, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO backup_key_ack (id, recipient, acknowledged_at, acknowledged_by) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET recipient = excluded.recipient, acknowledged_at = excluded.acknowledged_at, acknowledged_by = excluded.acknowledged_by`,
		recipient, now.UTC().Format(time.RFC3339Nano), actor)
	if err != nil {
		return fmt.Errorf("store: set backup key acknowledgement: %w", err)
	}
	return nil
}
