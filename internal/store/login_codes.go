package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrLoginCodeNotFound is returned when no live challenge matches.
var ErrLoginCodeNotFound = errors.New("store: login code challenge not found")

// Login code challenge states.
const (
	LoginCodePending  = "pending"
	LoginCodeRedeemed = "redeemed"
	LoginCodeLocked   = "locked"
	LoginCodeExpired  = "expired"
)

// Audit action names for sign in with a code and new-device approval.
const (
	AuditActionFamilyLoginCode   = "login_code"
	AuditActionLoginCodeRequest  = AuditActionFamilyLoginCode + ".requested"
	AuditActionLoginCodeDeliver  = AuditActionFamilyLoginCode + ".delivered"
	AuditActionLoginCodeRedeem   = AuditActionFamilyLoginCode + ".redeemed"
	AuditActionLoginCodeFail     = AuditActionFamilyLoginCode + ".failed"
	AuditActionLoginCodeLockout  = AuditActionFamilyLoginCode + ".locked"
	AuditActionLoginCodeExpire   = AuditActionFamilyLoginCode + ".expired"
	AuditActionFamilyNewDevice   = "new_device"
	AuditActionNewDeviceRequest  = AuditActionFamilyNewDevice + ".requested"
	AuditActionNewDeviceApprove  = AuditActionFamilyNewDevice + ".approved"
	AuditActionNewDeviceDeny     = AuditActionFamilyNewDevice + ".denied"
	AuditActionNewDeviceExpire   = AuditActionFamilyNewDevice + ".expired"
	AuditActionNewDeviceReplace  = AuditActionFamilyNewDevice + ".superseded"
	AuditActionTrustedDeviceDrop = AuditActionFamilyNewDevice + ".trust_revoked"
	AuditActionApproverRevoke    = AuditActionFamilyNewDevice + ".approver_tokens_revoked"
)

// LoginCodeChallenge is one "sign in with a code" request. CodeHash and Salt
// are the only form of the code ever persisted.
type LoginCodeChallenge struct {
	ID          string
	UserID      string
	BrowserHash string
	CodeHash    string
	Salt        string
	RequesterIP string
	UserAgent   string
	Status      string
	Attempts    int
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

const loginCodeColumns = `id, user_id, browser_hash, code_hash, salt, requester_ip, user_agent, status, attempts, created_at, expires_at`

func scanLoginCode(row interface{ Scan(dest ...any) error }) (LoginCodeChallenge, error) {
	var c LoginCodeChallenge
	var created, expires string
	if err := row.Scan(&c.ID, &c.UserID, &c.BrowserHash, &c.CodeHash, &c.Salt, &c.RequesterIP, &c.UserAgent,
		&c.Status, &c.Attempts, &created, &expires); err != nil {
		return c, err
	}
	c.CreatedAt = parseAuthTime(created)
	c.ExpiresAt = parseAuthTime(expires)
	return c, nil
}

func parseAuthTime(s string) time.Time {
	t, err := time.Parse(auditTimeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CreateLoginCodeChallenge inserts a new pending challenge.
func (db *DB) CreateLoginCodeChallenge(ctx context.Context, c LoginCodeChallenge) error {
	_, err := db.ExecContext(ctx, `INSERT INTO login_code_challenges (`+loginCodeColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		c.ID, c.UserID, c.BrowserHash, c.CodeHash, c.Salt, c.RequesterIP, c.UserAgent, LoginCodePending,
		FormatAuditTime(c.CreatedAt), FormatAuditTime(c.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: insert login code challenge: %w", err)
	}
	return nil
}

// GetLiveLoginCodeByBrowser returns the newest pending, unexpired challenge
// bound to browserHash.
func (db *DB) GetLiveLoginCodeByBrowser(ctx context.Context, browserHash string, now time.Time) (LoginCodeChallenge, error) {
	row := db.QueryRowContext(ctx, `SELECT `+loginCodeColumns+` FROM login_code_challenges
		WHERE browser_hash = ? AND status = ? AND expires_at > ? ORDER BY created_at DESC LIMIT 1`,
		browserHash, LoginCodePending, FormatAuditTime(now))
	c, err := scanLoginCode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrLoginCodeNotFound
	}
	if err != nil {
		return c, fmt.Errorf("store: get login code by browser: %w", err)
	}
	return c, nil
}

// ReserveLoginCodeAttempt spends one of a pending challenge's attempts before
// the code is compared, so parallel guesses can never exceed maxAttempts. It
// reports the attempt number, or ok false when the challenge is spent or gone.
func (db *DB) ReserveLoginCodeAttempt(ctx context.Context, id string, maxAttempts int) (attempt int, ok bool, err error) {
	row := db.QueryRowContext(ctx, `UPDATE login_code_challenges SET attempts = attempts + 1
		WHERE id = ? AND status = ? AND attempts < ? RETURNING attempts`, id, LoginCodePending, maxAttempts)
	if err := row.Scan(&attempt); errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	} else if err != nil {
		return 0, false, fmt.Errorf("store: reserve login code attempt %q: %w", id, err)
	}
	return attempt, true, nil
}

// LockLoginCode kills a pending challenge after its last failed attempt and
// reports whether this call did it.
func (db *DB) LockLoginCode(ctx context.Context, id string, now time.Time) (bool, error) {
	return db.moveLoginCode(ctx, id, LoginCodeLocked, now, false)
}

// RedeemLoginCode moves a pending, unexpired challenge to redeemed. It
// reports false when another request already resolved it, so a code is
// usable exactly once.
func (db *DB) RedeemLoginCode(ctx context.Context, id string, now time.Time) (bool, error) {
	return db.moveLoginCode(ctx, id, LoginCodeRedeemed, now, true)
}

// ExpireLoginCode retires a pending challenge early, and reports whether this
// call did it.
func (db *DB) ExpireLoginCode(ctx context.Context, id string, now time.Time) (bool, error) {
	return db.moveLoginCode(ctx, id, LoginCodeExpired, now, false)
}

// LockLoginCodesForUser locks every pending challenge of userID and returns
// the ids it moved, so a credential reset leaves no code redeemable.
func (db *DB) LockLoginCodesForUser(ctx context.Context, userID string, now time.Time) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `UPDATE login_code_challenges SET status = ?, resolved_at = ?
		WHERE user_id = ? AND status = ? RETURNING id`, LoginCodeLocked, FormatAuditTime(now), userID, LoginCodePending)
	if err != nil {
		return nil, fmt.Errorf("store: lock login codes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: lock login codes: scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (db *DB) moveLoginCode(ctx context.Context, id, to string, now time.Time, requireLive bool) (bool, error) {
	q := `UPDATE login_code_challenges SET status = ?, resolved_at = ? WHERE id = ? AND status = ?`
	args := []any{to, FormatAuditTime(now), id, LoginCodePending}
	if requireLive {
		q += ` AND expires_at > ?`
		args = append(args, FormatAuditTime(now))
	}
	res, err := db.ExecContext(ctx, q, args...)
	if err != nil {
		return false, fmt.Errorf("store: move login code %q to %s: %w", id, to, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: move login code %q: rows: %w", id, err)
	}
	return n == 1, nil
}

// ListLiveLoginCodesForUser returns userID's pending, unexpired challenges,
// newest first.
func (db *DB) ListLiveLoginCodesForUser(ctx context.Context, userID string, now time.Time) ([]LoginCodeChallenge, error) {
	if userID == "" {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT `+loginCodeColumns+` FROM login_code_challenges
		WHERE user_id = ? AND status = ? AND expires_at > ? ORDER BY created_at DESC`,
		userID, LoginCodePending, FormatAuditTime(now))
	if err != nil {
		return nil, fmt.Errorf("store: list login codes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LoginCodeChallenge
	for rows.Next() {
		c, err := scanLoginCode(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan login code: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ClaimLapsedLoginCodes marks up to limit pending challenges past their
// expiry as expired and returns exactly the ones this call moved.
func (db *DB) ClaimLapsedLoginCodes(ctx context.Context, now time.Time, limit int) ([]LoginCodeChallenge, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+loginCodeColumns+` FROM login_code_challenges
		WHERE status = ? AND expires_at <= ? ORDER BY expires_at LIMIT ?`, LoginCodePending, FormatAuditTime(now), limit)
	if err != nil {
		return nil, fmt.Errorf("store: list lapsed login codes: %w", err)
	}
	var lapsed []LoginCodeChallenge
	for rows.Next() {
		c, err := scanLoginCode(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: scan lapsed login code: %w", err)
		}
		lapsed = append(lapsed, c)
	}
	_ = rows.Close()
	var claimed []LoginCodeChallenge
	for _, c := range lapsed {
		ok, err := db.moveLoginCode(ctx, c.ID, LoginCodeExpired, now, false)
		if err != nil {
			return claimed, err
		}
		if ok {
			claimed = append(claimed, c)
		}
	}
	return claimed, nil
}

// PruneLoginCodes deletes resolved challenges and approvals older than cutoff.
func (db *DB) PruneLoginCodes(ctx context.Context, cutoff time.Time) error {
	ts := FormatAuditTime(cutoff)
	if _, err := db.ExecContext(ctx, `DELETE FROM login_code_challenges WHERE status != ? AND expires_at < ?`, LoginCodePending, ts); err != nil {
		return fmt.Errorf("store: prune login codes: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM login_approvals WHERE status != ? AND expires_at < ?`, LoginApprovalPending, ts); err != nil {
		return fmt.Errorf("store: prune login approvals: %w", err)
	}
	return nil
}
