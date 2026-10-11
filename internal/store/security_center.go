package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Audit action names for the security center.
const (
	AuditActionFamilySecurity        = "security"
	AuditActionSessionRevoke         = AuditActionFamilySecurity + ".session_revoked"
	AuditActionSessionRevokeOthers   = AuditActionFamilySecurity + ".sessions_revoked"
	AuditActionSecurityPolicyUpdate  = AuditActionFamilySecurity + ".policy_updated"
	AuditActionDeviceApprovalToggle  = AuditActionFamilySecurity + ".device_approval_changed"
	AuditActionNewBrowserSignIn      = AuditActionFamilySecurity + ".new_browser_sign_in"
	AuditActionSignInDisowned        = AuditActionFamilySecurity + ".sign_in_disowned"
	AuditActionTokenUnusedNotice     = AuditActionFamilySecurity + ".token_unused_notice"
	AuditActionTokenUnusedDisabled   = AuditActionFamilySecurity + ".token_unused_disabled"
	AuditActionFailedLoginsThreshold = AuditActionFamilySecurity + ".failed_logins"
)

// SecurityPolicy is the stored platform security policy. A nil field was
// never saved, so its APP_* env default applies.
type SecurityPolicy struct {
	ApprovalScope        *string
	MaxTokenLifetimeDays *int
	WarnUnusedDays       *int
	DisableUnusedDays    *int
	UpdatedAt            time.Time
}

// UserSecuritySettings are one account's own security switches.
type UserSecuritySettings struct {
	UserID                   string
	RequireNewDeviceApproval bool
	ResetFlaggedAt           time.Time
	ResetFlagReason          string
}

// SignInAlertToken backs one "this wasn't me" link from a new-browser alert.
type SignInAlertToken struct {
	ID              string
	UserID          string
	SessionID       string
	TrustedDeviceID string
	IP              string
	Label           string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// TokenHygieneNotice records that an unused token was warned about, and
// when it was disabled if the grace period ran out.
type TokenHygieneNotice struct {
	TokenID    string
	OwnerID    string
	TokenName  string
	NoticedAt  time.Time
	DisabledAt time.Time
}

// ErrSignInAlertTokenUsed covers an unknown, used or expired alert token.
var ErrSignInAlertTokenUsed = errors.New("store: sign-in alert token is invalid, used or expired")

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// GetSecurityPolicy returns the stored policy; ok is false when none was saved.
func (db *DB) GetSecurityPolicy(ctx context.Context) (p SecurityPolicy, ok bool, err error) {
	var scope sql.NullString
	var maxLife, warn, disable sql.NullInt64
	var updated string
	err = db.QueryRowContext(ctx, `SELECT approval_scope, max_token_lifetime_days, warn_unused_days, disable_unused_days, updated_at
		FROM security_policy_settings WHERE id = 1`).Scan(&scope, &maxLife, &warn, &disable, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, fmt.Errorf("store: get security policy: %w", err)
	}
	if scope.Valid {
		s := scope.String
		p.ApprovalScope = &s
	}
	p.MaxTokenLifetimeDays, p.WarnUnusedDays, p.DisableUnusedDays = intPtr(maxLife), intPtr(warn), intPtr(disable)
	p.UpdatedAt = parseAuthTime(updated)
	return p, true, nil
}

// SaveSecurityPolicy replaces the stored policy row.
func (db *DB) SaveSecurityPolicy(ctx context.Context, p SecurityPolicy) error {
	var scope sql.NullString
	if p.ApprovalScope != nil {
		scope = sql.NullString{String: *p.ApprovalScope, Valid: true}
	}
	_, err := db.ExecContext(ctx, `INSERT INTO security_policy_settings (id, approval_scope, max_token_lifetime_days, warn_unused_days, disable_unused_days, updated_at)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET approval_scope = excluded.approval_scope, max_token_lifetime_days = excluded.max_token_lifetime_days,
			warn_unused_days = excluded.warn_unused_days, disable_unused_days = excluded.disable_unused_days, updated_at = excluded.updated_at`,
		scope, nullInt(p.MaxTokenLifetimeDays), nullInt(p.WarnUnusedDays), nullInt(p.DisableUnusedDays), FormatAuditTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save security policy: %w", err)
	}
	return nil
}

const userSecurityColumns = `user_id, require_new_device_approval, reset_flagged_at, reset_flag_reason`

func scanUserSecurity(row interface{ Scan(dest ...any) error }) (UserSecuritySettings, error) {
	var s UserSecuritySettings
	var require int
	var flagged string
	if err := row.Scan(&s.UserID, &require, &flagged, &s.ResetFlagReason); err != nil {
		return s, err
	}
	s.RequireNewDeviceApproval = require == 1
	s.ResetFlaggedAt = parseAuthTime(flagged)
	return s, nil
}

// GetUserSecuritySettings returns userID's settings, zero valued when unset.
func (db *DB) GetUserSecuritySettings(ctx context.Context, userID string) (UserSecuritySettings, error) {
	s, err := scanUserSecurity(db.QueryRowContext(ctx, `SELECT `+userSecurityColumns+` FROM user_security_settings WHERE user_id = ?`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return UserSecuritySettings{UserID: userID}, nil
	}
	if err != nil {
		return s, fmt.Errorf("store: get user security settings: %w", err)
	}
	return s, nil
}

// ListUserSecuritySettings returns every account that has a stored row.
func (db *DB) ListUserSecuritySettings(ctx context.Context) ([]UserSecuritySettings, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+userSecurityColumns+` FROM user_security_settings ORDER BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list user security settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []UserSecuritySettings
	for rows.Next() {
		s, err := scanUserSecurity(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan user security settings: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetRequireNewDeviceApproval stores userID's own approval switch.
func (db *DB) SetRequireNewDeviceApproval(ctx context.Context, userID string, on bool, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO user_security_settings (user_id, require_new_device_approval, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET require_new_device_approval = excluded.require_new_device_approval, updated_at = excluded.updated_at`,
		userID, boolInt(on), FormatAuditTime(now))
	if err != nil {
		return fmt.Errorf("store: set require new device approval: %w", err)
	}
	return nil
}

// FlagUserForReset marks userID as needing a password reset.
func (db *DB) FlagUserForReset(ctx context.Context, userID, reason string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO user_security_settings (user_id, reset_flagged_at, reset_flag_reason, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET reset_flagged_at = excluded.reset_flagged_at, reset_flag_reason = excluded.reset_flag_reason, updated_at = excluded.updated_at`,
		userID, FormatAuditTime(now), reason, FormatAuditTime(now))
	if err != nil {
		return fmt.Errorf("store: flag user for reset: %w", err)
	}
	return nil
}

// ClearUserResetFlag drops the reset flag after a password change or reset.
func (db *DB) ClearUserResetFlag(ctx context.Context, userID string, now time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE user_security_settings SET reset_flagged_at = '', reset_flag_reason = '', updated_at = ? WHERE user_id = ?`,
		FormatAuditTime(now), userID)
	if err != nil {
		return fmt.Errorf("store: clear user reset flag: %w", err)
	}
	return nil
}

// CreateSignInAlertToken inserts an alert link row.
func (db *DB) CreateSignInAlertToken(ctx context.Context, t SignInAlertToken) error {
	_, err := db.ExecContext(ctx, `INSERT INTO sign_in_alert_tokens (id, user_id, session_id, trusted_device_id, ip, label, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, t.ID, t.UserID, t.SessionID, t.TrustedDeviceID, t.IP, t.Label,
		FormatAuditTime(t.CreatedAt), FormatAuditTime(t.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: insert sign-in alert token: %w", err)
	}
	return nil
}

// ConsumeSignInAlertToken marks id used and returns it, exactly once, and
// only before it expires. Anything else is ErrSignInAlertTokenUsed.
func (db *DB) ConsumeSignInAlertToken(ctx context.Context, id string, now time.Time) (SignInAlertToken, error) {
	var t SignInAlertToken
	res, err := db.ExecContext(ctx, `UPDATE sign_in_alert_tokens SET used_at = ? WHERE id = ? AND used_at = '' AND expires_at > ?`,
		FormatAuditTime(now), id, FormatAuditTime(now))
	if err != nil {
		return t, fmt.Errorf("store: consume sign-in alert token: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return t, ErrSignInAlertTokenUsed
	}
	var created, expires string
	err = db.QueryRowContext(ctx, `SELECT id, user_id, session_id, trusted_device_id, ip, label, created_at, expires_at FROM sign_in_alert_tokens WHERE id = ?`, id).
		Scan(&t.ID, &t.UserID, &t.SessionID, &t.TrustedDeviceID, &t.IP, &t.Label, &created, &expires)
	if err != nil {
		return t, fmt.Errorf("store: load sign-in alert token: %w", err)
	}
	t.CreatedAt, t.ExpiresAt = parseAuthTime(created), parseAuthTime(expires)
	return t, nil
}

// PruneSignInAlertTokens deletes alert tokens that expired before cutoff.
func (db *DB) PruneSignInAlertTokens(ctx context.Context, cutoff time.Time) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM sign_in_alert_tokens WHERE expires_at < ?`, FormatAuditTime(cutoff)); err != nil {
		return fmt.Errorf("store: prune sign-in alert tokens: %w", err)
	}
	return nil
}

// ListTokenHygieneNotices returns every recorded notice, oldest first.
func (db *DB) ListTokenHygieneNotices(ctx context.Context) ([]TokenHygieneNotice, error) {
	rows, err := db.QueryContext(ctx, `SELECT token_id, owner_id, token_name, noticed_at, disabled_at FROM token_hygiene_notices ORDER BY noticed_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list token hygiene notices: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TokenHygieneNotice
	for rows.Next() {
		var n TokenHygieneNotice
		var noticed, disabled string
		if err := rows.Scan(&n.TokenID, &n.OwnerID, &n.TokenName, &noticed, &disabled); err != nil {
			return nil, fmt.Errorf("store: scan token hygiene notice: %w", err)
		}
		n.NoticedAt, n.DisabledAt = parseAuthTime(noticed), parseAuthTime(disabled)
		out = append(out, n)
	}
	return out, rows.Err()
}

// SaveTokenHygieneNotice records a notice; an existing one keeps its time.
func (db *DB) SaveTokenHygieneNotice(ctx context.Context, n TokenHygieneNotice) error {
	_, err := db.ExecContext(ctx, `INSERT INTO token_hygiene_notices (token_id, owner_id, token_name, noticed_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (token_id) DO NOTHING`, n.TokenID, n.OwnerID, n.TokenName, FormatAuditTime(n.NoticedAt))
	if err != nil {
		return fmt.Errorf("store: save token hygiene notice: %w", err)
	}
	return nil
}

// MarkTokenHygieneDisabled records that the sweep revoked the token.
func (db *DB) MarkTokenHygieneDisabled(ctx context.Context, tokenID string, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE token_hygiene_notices SET disabled_at = ? WHERE token_id = ?`, FormatAuditTime(now), tokenID); err != nil {
		return fmt.Errorf("store: mark token hygiene disabled: %w", err)
	}
	return nil
}

// DeleteTokenHygieneNotice forgets a notice, for a token used again since.
func (db *DB) DeleteTokenHygieneNotice(ctx context.Context, tokenID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM token_hygiene_notices WHERE token_id = ?`, tokenID); err != nil {
		return fmt.Errorf("store: delete token hygiene notice: %w", err)
	}
	return nil
}

// RememberBrowser records that userID signed in from fingerprint. It reports
// whether the browser was new and whether the account had any known browser.
func (db *DB) RememberBrowser(ctx context.Context, userID, fingerprint string, now time.Time) (isNew, hadAny bool, err error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM known_browsers WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return false, false, fmt.Errorf("store: count known browsers: %w", err)
	}
	res, err := db.ExecContext(ctx, `INSERT INTO known_browsers (user_id, fingerprint, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, fingerprint) DO NOTHING`, userID, fingerprint, FormatAuditTime(now), FormatAuditTime(now))
	if err != nil {
		return false, false, fmt.Errorf("store: remember browser: %w", err)
	}
	if added, _ := res.RowsAffected(); added == 1 {
		return true, n > 0, nil
	}
	if _, err := db.ExecContext(ctx, `UPDATE known_browsers SET last_seen_at = ? WHERE user_id = ? AND fingerprint = ?`,
		FormatAuditTime(now), userID, fingerprint); err != nil {
		return false, true, fmt.Errorf("store: touch known browser: %w", err)
	}
	return false, true, nil
}

// ForgetBrowser drops one known browser, after "this wasn't me".
func (db *DB) ForgetBrowser(ctx context.Context, userID, fingerprint string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM known_browsers WHERE user_id = ? AND fingerprint = ?`, userID, fingerprint); err != nil {
		return fmt.Errorf("store: forget browser: %w", err)
	}
	return nil
}

// PruneKnownBrowsers forgets browsers not seen since cutoff.
func (db *DB) PruneKnownBrowsers(ctx context.Context, cutoff time.Time) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM known_browsers WHERE last_seen_at < ?`, FormatAuditTime(cutoff)); err != nil {
		return fmt.Errorf("store: prune known browsers: %w", err)
	}
	return nil
}
