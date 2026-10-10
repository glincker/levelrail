package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrTrustedDeviceNotFound is returned for an unknown, revoked or expired device.
var ErrTrustedDeviceNotFound = errors.New("store: trusted device not found")

// TrustedDevice is one browser a user approved for password sign-in. TokenHash
// is the SHA-256 of its cookie; the cookie value is never stored.
type TrustedDevice struct {
	ID         string
	UserID     string
	TokenHash  string
	Label      string
	IP         string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
}

const trustedDeviceColumns = `id, user_id, token_hash, label, ip, created_at, last_used_at, expires_at`

func scanTrustedDevice(row interface{ Scan(dest ...any) error }) (TrustedDevice, error) {
	var d TrustedDevice
	var created, used, expires string
	if err := row.Scan(&d.ID, &d.UserID, &d.TokenHash, &d.Label, &d.IP, &created, &used, &expires); err != nil {
		return d, err
	}
	d.CreatedAt = parseAuthTime(created)
	d.LastUsedAt = parseAuthTime(used)
	d.ExpiresAt = parseAuthTime(expires)
	return d, nil
}

// CreateTrustedDevice inserts a trusted device.
func (db *DB) CreateTrustedDevice(ctx context.Context, d TrustedDevice) error {
	_, err := db.ExecContext(ctx, `INSERT INTO trusted_devices (`+trustedDeviceColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.UserID, d.TokenHash, d.Label, d.IP, FormatAuditTime(d.CreatedAt), FormatAuditTime(d.LastUsedAt), FormatAuditTime(d.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: insert trusted device: %w", err)
	}
	return nil
}

// GetTrustedDeviceByHash returns the live (unrevoked, unexpired) device whose
// cookie hashes to tokenHash.
func (db *DB) GetTrustedDeviceByHash(ctx context.Context, tokenHash string, now time.Time) (TrustedDevice, error) {
	d, err := scanTrustedDevice(db.QueryRowContext(ctx, `SELECT `+trustedDeviceColumns+` FROM trusted_devices
		WHERE token_hash = ? AND revoked_at = '' AND expires_at > ?`, tokenHash, FormatAuditTime(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrTrustedDeviceNotFound
	}
	if err != nil {
		return d, fmt.Errorf("store: get trusted device: %w", err)
	}
	return d, nil
}

// TouchTrustedDevice records a sign-in from a trusted device.
func (db *DB) TouchTrustedDevice(ctx context.Context, id string, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE trusted_devices SET last_used_at = ? WHERE id = ?`, FormatAuditTime(now), id); err != nil {
		return fmt.Errorf("store: touch trusted device %q: %w", id, err)
	}
	return nil
}

// ListTrustedDevices returns userID's live trusted devices, newest first.
func (db *DB) ListTrustedDevices(ctx context.Context, userID string, now time.Time) ([]TrustedDevice, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+trustedDeviceColumns+` FROM trusted_devices
		WHERE user_id = ? AND revoked_at = '' AND expires_at > ? ORDER BY created_at DESC`, userID, FormatAuditTime(now))
	if err != nil {
		return nil, fmt.Errorf("store: list trusted devices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TrustedDevice
	for rows.Next() {
		d, err := scanTrustedDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan trusted device: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeTrustedDevice revokes one of userID's devices and reports whether a
// live device was revoked. Another user's device id reads as not found.
func (db *DB) RevokeTrustedDevice(ctx context.Context, userID, id string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE trusted_devices SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at = ''`,
		FormatAuditTime(now), id, userID)
	if err != nil {
		return false, fmt.Errorf("store: revoke trusted device %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: revoke trusted device %q: rows: %w", id, err)
	}
	return n == 1, nil
}

// RevokeAllTrustedDevices revokes every live device of userID and reports
// how many it revoked.
func (db *DB) RevokeAllTrustedDevices(ctx context.Context, userID string, now time.Time) (int, error) {
	res, err := db.ExecContext(ctx, `UPDATE trusted_devices SET revoked_at = ? WHERE user_id = ? AND revoked_at = ''`,
		FormatAuditTime(now), userID)
	if err != nil {
		return 0, fmt.Errorf("store: revoke trusted devices of %q: %w", userID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: revoke trusted devices of %q: rows: %w", userID, err)
	}
	return int(n), nil
}

// ExtendTrustedDevice records a sign-in and pushes a live device's expiry.
func (db *DB) ExtendTrustedDevice(ctx context.Context, id string, expiresAt, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE trusted_devices SET last_used_at = ?, expires_at = ? WHERE id = ? AND revoked_at = ''`,
		FormatAuditTime(now), FormatAuditTime(expiresAt), id); err != nil {
		return fmt.Errorf("store: extend trusted device %q: %w", id, err)
	}
	return nil
}

// PruneTrustedDevices deletes devices that expired or were revoked before cutoff.
func (db *DB) PruneTrustedDevices(ctx context.Context, cutoff time.Time) error {
	ts := FormatAuditTime(cutoff)
	if _, err := db.ExecContext(ctx, `DELETE FROM trusted_devices WHERE expires_at < ? OR (revoked_at != '' AND revoked_at < ?)`, ts, ts); err != nil {
		return fmt.Errorf("store: prune trusted devices: %w", err)
	}
	return nil
}

// CodeLoginSettings says whether sign in with a code is offered to admin
// (root) accounts and to every other account.
type CodeLoginSettings struct {
	Admins    bool
	Others    bool
	UpdatedAt time.Time
}

// GetCodeLoginSettings returns the saved settings, or ok false when an admin
// never saved any and the environment defaults apply.
func (db *DB) GetCodeLoginSettings(ctx context.Context) (s CodeLoginSettings, ok bool, err error) {
	var admins, others int
	var updated string
	err = db.QueryRowContext(ctx, `SELECT admins, others, updated_at FROM auth_code_login_settings WHERE id = 1`).Scan(&admins, &others, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	if err != nil {
		return s, false, fmt.Errorf("store: get code login settings: %w", err)
	}
	return CodeLoginSettings{Admins: admins == 1, Others: others == 1, UpdatedAt: parseAuthTime(updated)}, true, nil
}

// SaveCodeLoginSettings stores the settings row.
func (db *DB) SaveCodeLoginSettings(ctx context.Context, s CodeLoginSettings) error {
	_, err := db.ExecContext(ctx, `INSERT INTO auth_code_login_settings (id, admins, others, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET admins = excluded.admins, others = excluded.others, updated_at = excluded.updated_at`,
		boolInt(s.Admins), boolInt(s.Others), FormatAuditTime(s.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save code login settings: %w", err)
	}
	return nil
}
