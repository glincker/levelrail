package store

import (
	"context"
	"database/sql"
	"fmt"
)

// GetDeployURLCopiesMode returns the stored copies mode, or "" when none is set
// and the env seed applies.
func (db *DB) GetDeployURLCopiesMode(ctx context.Context) (string, error) {
	var m sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT copies_mode FROM deploy_url_settings WHERE id = 1`).Scan(&m); err != nil {
		return "", fmt.Errorf("store: get deploy url copies mode: %w", err)
	}
	return m.String, nil
}

// SetDeployURLCopiesMode stores mode; "" clears it so the env seed applies again.
// The caller validates the value, the table CHECK is the backstop.
func (db *DB) SetDeployURLCopiesMode(ctx context.Context, mode string) error {
	_, err := db.ExecContext(ctx, `UPDATE deploy_url_settings SET copies_mode = ? WHERE id = 1`,
		sql.NullString{String: mode, Valid: mode != ""})
	if err != nil {
		return fmt.Errorf("store: set deploy url copies mode: %w", err)
	}
	return nil
}
