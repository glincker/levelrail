package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// ReplaceUserRecoveryCodes atomically deletes every existing recovery
// code row for userID and inserts one row per hash in hashes. Used both
// at first confirm (internal/api's handleConfirmTwoFactor) and on an
// explicit regenerate: either way the full set is replaced, never
// merged, so a stale code from before a regenerate can never validate.
func (db *DB) ReplaceUserRecoveryCodes(ctx context.Context, userID string, hashes []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace recovery codes for user %q: begin tx: %w", userID, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: replace recovery codes for user %q: delete existing: %w", userID, err)
	}

	now := formatTime(time.Now())
	for _, hash := range hashes {
		id, err := randomRecoveryCodeID()
		if err != nil {
			return fmt.Errorf("store: replace recovery codes for user %q: %w", userID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_recovery_codes (id, user_id, code_hash, used_at, created_at)
			VALUES (?, ?, ?, NULL, ?)
		`, id, userID, hash, now); err != nil {
			return fmt.Errorf("store: replace recovery codes for user %q: insert: %w", userID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace recovery codes for user %q: commit: %w", userID, err)
	}
	return nil
}

// ConsumeUserRecoveryCode marks the row matching (userID, hash) used, if
// it exists and hasn't been used already, and reports whether it did:
// the one-shot, atomic "was this a valid unused code" check
// internal/api's login and disable flows both need. A matched-but-
// already-used row and no match at all are indistinguishable to the
// caller, both report false, since neither should validate a login.
func (db *DB) ConsumeUserRecoveryCode(ctx context.Context, userID, hash string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE user_recovery_codes SET used_at = ?
		WHERE user_id = ? AND code_hash = ? AND used_at IS NULL
	`, formatTime(time.Now()), userID, hash)
	if err != nil {
		return false, fmt.Errorf("store: consume recovery code for user %q: %w", userID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: consume recovery code for user %q: rows affected: %w", userID, err)
	}
	return n > 0, nil
}

// CountUnusedUserRecoveryCodes returns how many of userID's recovery
// codes have not been consumed yet, the settings-page "N codes
// remaining" figure.
func (db *DB) CountUnusedUserRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL
	`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count unused recovery codes for user %q: %w", userID, err)
	}
	return n, nil
}

// DeleteUserRecoveryCodes removes every recovery code row for userID,
// called when 2FA is disabled: a leftover valid recovery code would
// otherwise still grant access to an account whose owner just turned
// two-factor off.
func (db *DB) DeleteUserRecoveryCodes(ctx context.Context, userID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: delete recovery codes for user %q: %w", userID, err)
	}
	return nil
}

func randomRecoveryCodeID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate recovery code row id: %w", err)
	}
	return "rc_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
