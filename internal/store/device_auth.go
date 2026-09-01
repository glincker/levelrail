package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// DeviceAuthRequest is one CLI-login-from-terminal request (RFC
// 8628-shaped): the CLI polls DeviceCode while an operator approves
// UserCode from the web dashboard. See migrations/0079's own comment
// for the status lifecycle.
type DeviceAuthRequest struct {
	ID               string
	DeviceCode       string
	UserCode         string
	Status           string
	ClientName       string
	ApprovedByUserID *string
	TokenID          *string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	RedeemedAt       *time.Time
}

// DeviceAuthStatusPending, DeviceAuthStatusApproved, and
// DeviceAuthStatusDenied are the only valid DeviceAuthRequest.Status
// values.
const (
	DeviceAuthStatusPending  = "pending"
	DeviceAuthStatusApproved = "approved"
	DeviceAuthStatusDenied   = "denied"
)

// ErrDeviceAuthRequestNotFound is returned by GetDeviceAuthRequestByDeviceCode/
// GetDeviceAuthRequestByUserCode when no row matches.
var ErrDeviceAuthRequestNotFound = errors.New("store: device auth request not found")

// NewDeviceAuthRequestID mints a random device auth request ID, the
// same crypto/rand-plus-base64 scheme NewDeployAttemptID/NewPolicyID
// already establish.
func NewDeviceAuthRequestID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate device auth request id: %w", err)
	}
	return "dar_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewDeviceCode mints a long, opaque, unguessable code the CLI polls
// with: this is the credential half of the pair, never shown to a
// human, so it can be as dense as a token.
func NewDeviceCode() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate device code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// userCodeAlphabet excludes visually ambiguous characters (0/O, 1/I/L)
// so a human can read a code off a terminal and type it correctly.
const userCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// NewUserCode mints an 8-character, dash-grouped code a human types
// into the web UI (GitHub/gh CLI's own device-flow UX), e.g.
// "WDJB-MJHT". Short and low-entropy by design: it only needs to
// resist a casual guess for the few minutes it's valid, DeviceCode is
// what actually authorizes the token exchange.
func NewUserCode() (string, error) {
	b := make([]byte, 8)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(userCodeAlphabet))))
		if err != nil {
			return "", fmt.Errorf("store: generate user code: %w", err)
		}
		b[i] = userCodeAlphabet[n.Int64()]
	}
	return string(b[:4]) + "-" + string(b[4:]), nil
}

// SaveDeviceAuthRequest inserts a new pending device auth request.
func (db *DB) SaveDeviceAuthRequest(ctx context.Context, r DeviceAuthRequest) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO device_auth_requests (id, device_code, user_code, status, client_name, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.DeviceCode, r.UserCode, DeviceAuthStatusPending, r.ClientName, formatTime(r.CreatedAt), formatTime(r.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: save device auth request %q: %w", r.ID, err)
	}
	return nil
}

// GetDeviceAuthRequestByDeviceCode is the CLI poll's own lookup.
func (db *DB) GetDeviceAuthRequestByDeviceCode(ctx context.Context, deviceCode string) (*DeviceAuthRequest, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, device_code, user_code, status, client_name, approved_by_user_id, token_id, created_at, expires_at, redeemed_at
		FROM device_auth_requests WHERE device_code = ?
	`, deviceCode)
	return scanDeviceAuthRequest(row.Scan)
}

// GetDeviceAuthRequestByUserCode is the web UI's own lookup, keyed by
// the code an operator types in.
func (db *DB) GetDeviceAuthRequestByUserCode(ctx context.Context, userCode string) (*DeviceAuthRequest, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, device_code, user_code, status, client_name, approved_by_user_id, token_id, created_at, expires_at, redeemed_at
		FROM device_auth_requests WHERE user_code = ?
	`, userCode)
	return scanDeviceAuthRequest(row.Scan)
}

// ListPendingDeviceAuthRequests returns every still-pending,
// not-yet-expired request, oldest first: the web UI's approval queue.
func (db *DB) ListPendingDeviceAuthRequests(ctx context.Context, now time.Time) ([]DeviceAuthRequest, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, device_code, user_code, status, client_name, approved_by_user_id, token_id, created_at, expires_at, redeemed_at
		FROM device_auth_requests WHERE status = ? AND expires_at > ? ORDER BY created_at
	`, DeviceAuthStatusPending, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("store: list pending device auth requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []DeviceAuthRequest
	for rows.Next() {
		r, err := scanDeviceAuthRequest(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan device auth request row: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate device auth request rows: %w", err)
	}
	return out, nil
}

// SetDeviceAuthRequestStatus moves a pending request to approved or
// denied, recording who approved it (empty for a denial). Only
// affects a row still in "pending" status, so a request already
// decided (or already redeemed) can't be flipped again; the caller
// distinguishes "no such code" from "already decided" by re-reading
// afterward if it needs to.
func (db *DB) SetDeviceAuthRequestStatus(ctx context.Context, userCode, status, approvedByUserID string) (int64, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE device_auth_requests SET status = ?, approved_by_user_id = ?
		WHERE user_code = ? AND status = ?
	`, status, nullableString(nilIfEmpty(approvedByUserID)), userCode, DeviceAuthStatusPending)
	if err != nil {
		return 0, fmt.Errorf("store: set device auth request %q status: %w", userCode, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: set device auth request %q status: %w", userCode, err)
	}
	return n, nil
}

// RedeemDeviceAuthRequest marks an approved request as redeemed,
// recording the freshly minted token's ID: this is what makes a
// device_code single-use, since the CLI poll handler only mints a
// token when redeemed_at is still unset.
func (db *DB) RedeemDeviceAuthRequest(ctx context.Context, deviceCode, tokenID string, redeemedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE device_auth_requests SET token_id = ?, redeemed_at = ?
		WHERE device_code = ? AND redeemed_at IS NULL
	`, tokenID, formatTime(redeemedAt), deviceCode)
	if err != nil {
		return fmt.Errorf("store: redeem device auth request: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: redeem device auth request: %w", err)
	}
	if n == 0 {
		return ErrDeviceAuthRequestNotFound
	}
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanDeviceAuthRequest(scan func(dest ...any) error) (*DeviceAuthRequest, error) {
	var r DeviceAuthRequest
	var approvedBy, tokenID sql.NullString
	var createdAt, expiresAt string
	var redeemedAt sql.NullString
	if err := scan(&r.ID, &r.DeviceCode, &r.UserCode, &r.Status, &r.ClientName, &approvedBy, &tokenID, &createdAt, &expiresAt, &redeemedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeviceAuthRequestNotFound
		}
		return nil, err
	}
	if approvedBy.Valid {
		r.ApprovedByUserID = &approvedBy.String
	}
	if tokenID.Valid {
		r.TokenID = &tokenID.String
	}
	ct, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	r.CreatedAt = ct
	et, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	r.ExpiresAt = et
	if redeemedAt.Valid {
		rt, err := time.Parse(time.RFC3339Nano, redeemedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse redeemed_at: %w", err)
		}
		r.RedeemedAt = &rt
	}
	return &r, nil
}
