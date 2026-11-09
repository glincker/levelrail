package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AppExpiry is a scheduled stop and later delete of one app (migrations/3228).
type AppExpiry struct {
	AppName    string `json:"app_name"`
	ExpiresAt  string `json:"expires_at"`
	DeleteData bool   `json:"delete_data"`
	SetBy      string `json:"set_by,omitempty"`
	SetAt      string `json:"set_at"`
	NotifiedAt string `json:"notified_at,omitempty"`
	StoppedAt  string `json:"stopped_at,omitempty"`
	KeptReason string `json:"kept_reason,omitempty"`
	// Incarnation is the service life the expiry was set on; another life never inherits it.
	Incarnation string `json:"-"`
	// SetterType and SetterID name who set it, re-checked before a delete_data delete.
	SetterType string `json:"-"`
	SetterID   string `json:"-"`
	// StoppedBy is "sweep" or "already_stopped"; ClaimedAt marks a delete in progress.
	StoppedBy string `json:"stopped_by,omitempty"`
	ClaimedAt string `json:"claimed_at,omitempty"`
}

// AppExpiryKeptStarted is the kept reason for an app started after its expiry stop.
const AppExpiryKeptStarted = "started after expiry"

const appExpiryColumns = `app_name, expires_at, delete_data, set_by, set_at, notified_at, stopped_at, kept_reason, incarnation, setter_type, setter_id, stopped_by, claimed_at`

func scanAppExpiry(scan func(...any) error) (AppExpiry, error) {
	var e AppExpiry
	err := scan(&e.AppName, &e.ExpiresAt, &e.DeleteData, &e.SetBy, &e.SetAt, &e.NotifiedAt, &e.StoppedAt, &e.KeptReason, &e.Incarnation, &e.SetterType, &e.SetterID, &e.StoppedBy, &e.ClaimedAt)
	return e, err
}

// GetAppExpiry returns the app's expiry, or nil when none is set.
func (db *DB) GetAppExpiry(ctx context.Context, app string) (*AppExpiry, error) {
	e, err := scanAppExpiry(db.QueryRowContext(ctx, `SELECT `+appExpiryColumns+` FROM app_expiry WHERE app_name = ?`, app).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get app expiry: %w", err)
	}
	return &e, nil
}

// SetAppExpiry saves e and resets its progress, so a new date starts a fresh notice and stop.
func (db *DB) SetAppExpiry(ctx context.Context, e AppExpiry) error {
	res, err := db.ExecContext(ctx, `INSERT INTO app_expiry (`+appExpiryColumns+`) VALUES (?, ?, ?, ?, ?, '', '', '', ?, ?, ?, '', '')
		ON CONFLICT(app_name) DO UPDATE SET expires_at = excluded.expires_at, delete_data = excluded.delete_data,
		set_by = excluded.set_by, set_at = excluded.set_at, notified_at = '', stopped_at = '', kept_reason = '',
		incarnation = excluded.incarnation, setter_type = excluded.setter_type, setter_id = excluded.setter_id,
		stopped_by = '', claimed_at = ''
		WHERE app_expiry.claimed_at = ''`,
		e.AppName, e.ExpiresAt, e.DeleteData, e.SetBy, e.SetAt, e.Incarnation, e.SetterType, e.SetterID)
	if err != nil {
		return fmt.Errorf("store: set app expiry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAppExpiryClaimed
	}
	return nil
}

// ErrAppExpiryClaimed means a delete holds the row, so it cannot be set again now.
var ErrAppExpiryClaimed = errors.New("store: app expiry is claimed by a running delete")

// KeepRestoredAppExpiries marks already stopped expiries in a restored database
// file kept, so an old stop stamp never turns into an immediate delete.
func KeepRestoredAppExpiries(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return fmt.Errorf("store: open restored database: %w", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'app_expiry'`).Scan(&n); err != nil {
		return fmt.Errorf("store: inspect restored app expiries: %w", err)
	}
	if n == 0 {
		return nil
	}
	if _, err := db.ExecContext(ctx, `UPDATE app_expiry SET kept_reason = 'restored from backup', claimed_at = '' WHERE stopped_at != '' AND kept_reason = ''`); err != nil {
		return fmt.Errorf("store: keep restored app expiries: %w", err)
	}
	return nil
}

// ClearAppExpiry removes the expiry and reports whether one existed.
func (db *DB) ClearAppExpiry(ctx context.Context, app string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM app_expiry WHERE app_name = ?`, app)
	if err != nil {
		return false, fmt.Errorf("store: clear app expiry: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListAppExpiries returns every expiry, soonest first.
func (db *DB) ListAppExpiries(ctx context.Context) ([]AppExpiry, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+appExpiryColumns+` FROM app_expiry ORDER BY expires_at ASC, app_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list app expiries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppExpiry
	for rows.Next() {
		e, err := scanAppExpiry(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan app expiry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkAppExpiry records sweep progress on the exact unclaimed row e was read
// from; it reports false when the row was cleared or set again in the meantime.
func (db *DB) MarkAppExpiry(ctx context.Context, e AppExpiry, notifiedAt, stoppedAt, stoppedBy, keptReason string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE app_expiry SET notified_at = ?, stopped_at = ?, stopped_by = ?, kept_reason = ?
		WHERE app_name = ? AND expires_at = ? AND set_at = ? AND incarnation = ? AND stopped_at = ? AND kept_reason = ? AND claimed_at = ''`,
		notifiedAt, stoppedAt, stoppedBy, keptReason, e.AppName, e.ExpiresAt, e.SetAt, e.Incarnation, e.StoppedAt, e.KeptReason)
	if err != nil {
		return false, fmt.Errorf("store: mark app expiry: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// UnstampAppExpiryStop rolls back a stop stamp whose suspend did not happen.
func (db *DB) UnstampAppExpiryStop(ctx context.Context, app, stoppedAt string) error {
	_, err := db.ExecContext(ctx, `UPDATE app_expiry SET stopped_at = '', stopped_by = '' WHERE app_name = ? AND stopped_at = ? AND claimed_at = ''`, app, stoppedAt)
	if err != nil {
		return fmt.Errorf("store: unstamp app expiry stop: %w", err)
	}
	return nil
}

// ClaimAppExpiry stamps claimed_at only when every column still matches e and
// no claim exists, so exactly one caller wins and a clear in between cancels it.
func (db *DB) ClaimAppExpiry(ctx context.Context, e AppExpiry, claimedAt string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE app_expiry SET claimed_at = ? WHERE app_name = ? AND expires_at = ? AND set_at = ?
		AND delete_data = ? AND notified_at = ? AND stopped_at = ? AND stopped_by = ? AND kept_reason = ? AND incarnation = ?
		AND setter_type = ? AND setter_id = ? AND claimed_at = ''`,
		claimedAt, e.AppName, e.ExpiresAt, e.SetAt, e.DeleteData, e.NotifiedAt, e.StoppedAt, e.StoppedBy, e.KeptReason, e.Incarnation,
		e.SetterType, e.SetterID)
	if err != nil {
		return false, fmt.Errorf("store: claim app expiry: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ReleaseAppExpiryClaim drops a claim whose delete did not happen; a row
// cleared in the meantime stays gone because nothing is inserted.
func (db *DB) ReleaseAppExpiryClaim(ctx context.Context, app, claimedAt string) error {
	_, err := db.ExecContext(ctx, `UPDATE app_expiry SET claimed_at = '' WHERE app_name = ? AND claimed_at = ?`, app, claimedAt)
	if err != nil {
		return fmt.Errorf("store: release app expiry claim: %w", err)
	}
	return nil
}

// RemoveAppExpiryRow deletes a row only if it is still exactly e, for a stale row from another app life.
func (db *DB) RemoveAppExpiryRow(ctx context.Context, e AppExpiry) error {
	_, err := db.ExecContext(ctx, `DELETE FROM app_expiry WHERE app_name = ? AND set_at = ? AND incarnation = ? AND claimed_at = ''`,
		e.AppName, e.SetAt, e.Incarnation)
	if err != nil {
		return fmt.Errorf("store: remove app expiry row: %w", err)
	}
	return nil
}

// keepStartedAppExpiry marks a stopped expiry kept when the app is started again,
// whatever path started it, so a later unrelated suspend never leads to a delete.
func (db *DB) keepStartedAppExpiry(ctx context.Context, app string) error {
	_, err := db.ExecContext(ctx, `UPDATE app_expiry SET kept_reason = ? WHERE app_name = ? AND stopped_at != '' AND kept_reason = ''`,
		AppExpiryKeptStarted, app)
	if err != nil {
		return fmt.Errorf("store: keep started app expiry: %w", err)
	}
	return nil
}
