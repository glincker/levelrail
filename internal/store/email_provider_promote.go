package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PromoteEmailProvider makes the named backup the primary and the old primary the backup, in one transaction, and
// mirrors the new primary into email_settings. Credentials are not moved here; the caller owns that.
func (db *DB) PromoteEmailProvider(ctx context.Context, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	target, err := scanEmailProvider(tx.QueryRowContext(ctx, `SELECT `+emailProviderColumns+` FROM email_providers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEmailProviderNotFound
	}
	if err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}
	if target.Role == EmailRolePrimary {
		return invalidProvider("provider %q is already the primary", id)
	}
	old, err := scanEmailProvider(tx.QueryRowContext(ctx, `SELECT `+emailProviderColumns+` FROM email_providers WHERE role = 'primary'`))
	if errors.Is(err, sql.ErrNoRows) {
		return invalidProvider("there is no primary to swap with")
	}
	if err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}

	// The unique role index is checked per row, so swapping by UPDATE would collide: remove the old primary, promote, re-add it.
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_providers WHERE id = ?`, old.ID); err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE email_providers SET role = 'primary', updated_at = ? WHERE id = ?`, now, target.ID); err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}
	enabled, skip := 0, 0
	if old.Enabled {
		enabled = 1
	}
	if old.TLSSkipVerify {
		skip = 1
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO email_providers (`+emailProviderColumns+`)
		VALUES (?, 'backup', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, old.ID, old.Name, enabled, old.Backend, old.SMTPHost, old.SMTPPort, old.SMTPUsername, old.SMTPFrom,
		old.SESRegion, old.SESAccessKeyID, old.SESFrom, old.ResendFrom, old.TLSMode, skip,
		old.SESSMTPPort, old.SESSMTPUsername, old.MailgunDomain, old.MailgunRegion,
		formatProviderTime(old.CreatedAt), now, formatProviderTime(old.LastOKAt), old.LastErrorKind)
	if err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}
	if err := syncPrimaryToSettings(ctx, tx, &target); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: promote email provider: %w", err)
	}
	return nil
}
