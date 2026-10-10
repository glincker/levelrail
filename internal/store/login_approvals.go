package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrLoginApprovalNotFound is returned for an unknown approval or browser.
var ErrLoginApprovalNotFound = errors.New("store: login approval not found")

// Login approval states.
const (
	LoginApprovalPending  = "pending"
	LoginApprovalApproved = "approved"
	LoginApprovalDenied   = "denied"
	LoginApprovalExpired  = "expired"
	LoginApprovalConsumed = "consumed"
	// LoginApprovalSuperseded marks a request retired by a newer one.
	LoginApprovalSuperseded = "superseded"
)

// SupersedeLoginApprovals retires every pending or approved-but-unused
// approval of userID, so none can still collect a session, and returns the
// ones this call moved.
func (db *DB) SupersedeLoginApprovals(ctx context.Context, userID string, now time.Time) ([]LoginApproval, error) {
	var moved []LoginApproval
	err := db.inSignInTx(ctx, func(tx *sql.Tx) error {
		var err error
		moved, err = supersedeLoginApprovals(ctx, tx, userID, now)
		return err
	})
	return moved, err
}

// ReplaceLoginApproval supersedes a.UserID's open approvals and inserts a as
// the new pending one in one transaction, so at most one ever waits.
func (db *DB) ReplaceLoginApproval(ctx context.Context, a LoginApproval) ([]LoginApproval, error) {
	var moved []LoginApproval
	err := db.inSignInTx(ctx, func(tx *sql.Tx) error {
		var err error
		if moved, err = supersedeLoginApprovals(ctx, tx, a.UserID, a.CreatedAt); err != nil {
			return err
		}
		return insertLoginApproval(ctx, tx, a)
	})
	return moved, err
}

func (db *DB) inSignInTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit tx: %w", err)
	}
	return nil
}

func supersedeLoginApprovals(ctx context.Context, tx *sql.Tx, userID string, now time.Time) ([]LoginApproval, error) {
	open, err := queryLoginApprovals(ctx, tx, `SELECT `+loginApprovalColumns+` FROM login_approvals
		WHERE user_id = ? AND status IN (?, ?)`, userID, LoginApprovalPending, LoginApprovalApproved)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE login_approvals SET status = ?, resolved_at = ? WHERE user_id = ? AND status IN (?, ?)`,
		LoginApprovalSuperseded, FormatAuditTime(now), userID, LoginApprovalPending, LoginApprovalApproved); err != nil {
		return nil, fmt.Errorf("store: supersede login approvals: %w", err)
	}
	return open, nil
}

// LoginApproval is a password sign-in from an unrecognized browser, paused
// until one of the account's existing sessions approves or denies it.
type LoginApproval struct {
	ID          string
	UserID      string
	BrowserHash string
	RequesterIP string
	UserAgent   string
	Status      string
	DecidedBy   string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

const loginApprovalColumns = `id, user_id, browser_hash, requester_ip, user_agent, status, decided_by, created_at, expires_at`

func scanLoginApproval(row interface{ Scan(dest ...any) error }) (LoginApproval, error) {
	var a LoginApproval
	var created, expires string
	if err := row.Scan(&a.ID, &a.UserID, &a.BrowserHash, &a.RequesterIP, &a.UserAgent, &a.Status, &a.DecidedBy, &created, &expires); err != nil {
		return a, err
	}
	a.CreatedAt = parseAuthTime(created)
	a.ExpiresAt = parseAuthTime(expires)
	return a, nil
}

// CreateLoginApproval inserts a pending approval.
func (db *DB) CreateLoginApproval(ctx context.Context, a LoginApproval) error {
	return insertLoginApproval(ctx, db, a)
}

func insertLoginApproval(ctx context.Context, ex execer, a LoginApproval) error {
	_, err := ex.ExecContext(ctx, `INSERT INTO login_approvals (`+loginApprovalColumns+`) VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`,
		a.ID, a.UserID, a.BrowserHash, a.RequesterIP, a.UserAgent, LoginApprovalPending,
		FormatAuditTime(a.CreatedAt), FormatAuditTime(a.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: insert login approval: %w", err)
	}
	return nil
}

// GetLoginApproval loads one approval by id.
func (db *DB) GetLoginApproval(ctx context.Context, id string) (LoginApproval, error) {
	a, err := scanLoginApproval(db.QueryRowContext(ctx, `SELECT `+loginApprovalColumns+` FROM login_approvals WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrLoginApprovalNotFound
	}
	if err != nil {
		return a, fmt.Errorf("store: get login approval %q: %w", id, err)
	}
	return a, nil
}

// GetLoginApprovalByBrowser returns the newest approval bound to browserHash.
func (db *DB) GetLoginApprovalByBrowser(ctx context.Context, browserHash string) (LoginApproval, error) {
	a, err := scanLoginApproval(db.QueryRowContext(ctx, `SELECT `+loginApprovalColumns+` FROM login_approvals
		WHERE browser_hash = ? ORDER BY created_at DESC LIMIT 1`, browserHash))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrLoginApprovalNotFound
	}
	if err != nil {
		return a, fmt.Errorf("store: get login approval by browser: %w", err)
	}
	return a, nil
}

// DecideLoginApproval approves or denies a pending, unexpired approval owned
// by userID and reports whether this call decided it.
func (db *DB) DecideLoginApproval(ctx context.Context, id, userID string, approve bool, decidedBy string, now time.Time) (bool, error) {
	to := LoginApprovalDenied
	if approve {
		to = LoginApprovalApproved
	}
	res, err := db.ExecContext(ctx, `UPDATE login_approvals SET status = ?, decided_by = ?, resolved_at = ?
		WHERE id = ? AND user_id = ? AND status = ? AND expires_at > ?`,
		to, decidedBy, FormatAuditTime(now), id, userID, LoginApprovalPending, FormatAuditTime(now))
	if err != nil {
		return false, fmt.Errorf("store: decide login approval %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: decide login approval %q: rows: %w", id, err)
	}
	return n == 1, nil
}

// ConsumeLoginApproval moves an approved approval to consumed, once, so the
// session it unlocks is issued exactly once. Approval must still be unexpired.
func (db *DB) ConsumeLoginApproval(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE login_approvals SET status = ?, resolved_at = ?
		WHERE id = ? AND status = ? AND expires_at > ?`,
		LoginApprovalConsumed, FormatAuditTime(now), id, LoginApprovalApproved, FormatAuditTime(now))
	if err != nil {
		return false, fmt.Errorf("store: consume login approval %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: consume login approval %q: rows: %w", id, err)
	}
	return n == 1, nil
}

// ListPendingLoginApprovalsForUser returns userID's pending, unexpired
// approvals, newest first.
func (db *DB) ListPendingLoginApprovalsForUser(ctx context.Context, userID string, now time.Time) ([]LoginApproval, error) {
	return db.queryLoginApprovals(ctx, `SELECT `+loginApprovalColumns+` FROM login_approvals
		WHERE user_id = ? AND status = ? AND expires_at > ? ORDER BY created_at DESC`,
		userID, LoginApprovalPending, FormatAuditTime(now))
}

func (db *DB) queryLoginApprovals(ctx context.Context, q string, args ...any) ([]LoginApproval, error) {
	return queryLoginApprovals(ctx, db, q, args...)
}

type rowsQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryLoginApprovals(ctx context.Context, q rowsQuerier, query string, args ...any) ([]LoginApproval, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list login approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LoginApproval
	for rows.Next() {
		a, err := scanLoginApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan login approval: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ClaimLapsedLoginApprovals marks up to limit pending or approved-but-unused
// approvals past their expiry as expired, returning the ones this call moved.
func (db *DB) ClaimLapsedLoginApprovals(ctx context.Context, now time.Time, limit int) ([]LoginApproval, error) {
	lapsed, err := db.queryLoginApprovals(ctx, `SELECT `+loginApprovalColumns+` FROM login_approvals
		WHERE status IN (?, ?) AND expires_at <= ? ORDER BY expires_at LIMIT ?`,
		LoginApprovalPending, LoginApprovalApproved, FormatAuditTime(now), limit)
	if err != nil {
		return nil, err
	}
	var claimed []LoginApproval
	for _, a := range lapsed {
		res, err := db.ExecContext(ctx, `UPDATE login_approvals SET status = ?, resolved_at = ? WHERE id = ? AND status = ?`,
			LoginApprovalExpired, FormatAuditTime(now), a.ID, a.Status)
		if err != nil {
			return claimed, fmt.Errorf("store: expire login approval %q: %w", a.ID, err)
		}
		if n, _ := res.RowsAffected(); n == 1 {
			claimed = append(claimed, a)
		}
	}
	return claimed, nil
}
