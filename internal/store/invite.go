package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Invite is one outstanding team invite (migrations/0086). TokenHash is
// the only form of the invite token ever persisted, same convention as
// PasswordResetToken.TokenHash. Role is the curated preset name
// (internal/api/roles.go) the invite was created with, empty when
// Abilities was hand-picked instead; accepting the invite always applies
// Abilities, the already-resolved set either way.
type Invite struct {
	ID         string
	Email      string
	Role       string
	Abilities  []string
	TokenHash  string
	CreatedBy  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	RevokedAt  *time.Time
}

// ErrInviteNotFound is returned by GetInviteByHash/GetInviteByID when no
// row matches.
var ErrInviteNotFound = errors.New("store: invite not found")

// ErrInviteEmailExists is SaveInvite's failure mode when a pending
// (not yet accepted or revoked) invite already exists for the same
// email (ux_invites_pending_email).
var ErrInviteEmailExists = errors.New("store: a pending invite already exists for this email")

// ErrInviteAlreadyAccepted is RevokeInvite/ClaimInvite's failure mode
// when the invite was already accepted.
var ErrInviteAlreadyAccepted = errors.New("store: invite already accepted")

// ErrInviteAlreadyRevoked is RevokeInvite/ClaimInvite's failure mode
// when the invite was already revoked.
var ErrInviteAlreadyRevoked = errors.New("store: invite already revoked")

// nullEmptyString treats an empty string as NULL, for created_by: an API
// token (not backed by a users row) can create an invite too, and that
// case has no real user ID to store, only a nullable FK can hold both.
func nullEmptyString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// SaveInvite inserts a new invite row.
func (db *DB) SaveInvite(ctx context.Context, inv Invite) error {
	abilitiesJSON, err := json.Marshal(nonNilSlice(inv.Abilities))
	if err != nil {
		return fmt.Errorf("store: marshal abilities for invite %q: %w", inv.ID, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO invites (id, email, role, abilities, token_hash, created_by, created_at, expires_at, accepted_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, inv.ID, inv.Email, inv.Role, string(abilitiesJSON), inv.TokenHash, nullEmptyString(inv.CreatedBy),
		formatTime(inv.CreatedAt), formatTime(inv.ExpiresAt), formatTimePtr(inv.AcceptedAt), formatTimePtr(inv.RevokedAt))
	if err == nil {
		return nil
	}
	if _, getErr := db.getPendingInviteByEmail(ctx, inv.Email); getErr == nil {
		return ErrInviteEmailExists
	}
	return fmt.Errorf("store: save invite %q: %w", inv.ID, err)
}

const inviteColumns = `id, email, role, abilities, token_hash, created_by, created_at, expires_at, accepted_at, revoked_at`

func scanInvite(scan func(...any) error) (*Invite, error) {
	var (
		inv           Invite
		abilitiesJSON string
		createdBy     sql.NullString
		createdAt     string
		expiresAt     string
		acceptedAt    sql.NullString
		revokedAt     sql.NullString
	)
	if err := scan(&inv.ID, &inv.Email, &inv.Role, &abilitiesJSON, &inv.TokenHash, &createdBy,
		&createdAt, &expiresAt, &acceptedAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("store: scan invite: %w", err)
	}
	inv.CreatedBy = createdBy.String

	if err := json.Unmarshal([]byte(abilitiesJSON), &inv.Abilities); err != nil {
		return nil, fmt.Errorf("store: unmarshal invite abilities: %w", err)
	}
	var err error
	inv.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse invite created_at: %w", err)
	}
	inv.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse invite expires_at: %w", err)
	}
	inv.AcceptedAt, err = parseTimePtr(acceptedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse invite accepted_at: %w", err)
	}
	inv.RevokedAt, err = parseTimePtr(revokedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse invite revoked_at: %w", err)
	}
	return &inv, nil
}

// GetInviteByHash returns the invite matching hash, regardless of
// expiry/accepted/revoked state: deciding whether it's currently usable
// is the caller's job, matching GetPasswordResetTokenByHash's own shape.
func (db *DB) GetInviteByHash(ctx context.Context, hash string) (*Invite, error) {
	row := db.QueryRowContext(ctx, `SELECT `+inviteColumns+` FROM invites WHERE token_hash = ?`, hash)
	return scanInvite(row.Scan)
}

// GetInviteByID returns the invite with this ID, or ErrInviteNotFound.
func (db *DB) GetInviteByID(ctx context.Context, id string) (*Invite, error) {
	row := db.QueryRowContext(ctx, `SELECT `+inviteColumns+` FROM invites WHERE id = ?`, id)
	return scanInvite(row.Scan)
}

func (db *DB) getPendingInviteByEmail(ctx context.Context, email string) (*Invite, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+inviteColumns+` FROM invites
		WHERE email = ? AND accepted_at IS NULL AND revoked_at IS NULL
	`, email)
	return scanInvite(row.Scan)
}

// ListPendingInvites returns every invite not yet accepted or revoked,
// oldest first, including ones past their expires_at: an expired invite
// still shows up so an operator can see and revoke it, expiry itself is
// a display-time computation over ExpiresAt, not a filter here.
func (db *DB) ListPendingInvites(ctx context.Context) ([]Invite, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+inviteColumns+` FROM invites
		WHERE accepted_at IS NULL AND revoked_at IS NULL
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list pending invites: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []Invite
	for rows.Next() {
		inv, err := scanInvite(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan invite row: %w", err)
		}
		out = append(out, *inv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate invite rows: %w", err)
	}
	return out, nil
}

// RevokeInvite marks the named invite revoked, only if it hasn't already
// been accepted or revoked. Returns ErrInviteNotFound,
// ErrInviteAlreadyAccepted, or ErrInviteAlreadyRevoked to say exactly why
// nothing changed, rather than a single ambiguous failure.
func (db *DB) RevokeInvite(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE invites SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("store: revoke invite %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: revoke invite %q: rows affected: %w", id, err)
	}
	if n > 0 {
		return nil
	}
	return db.classifyInviteMutationFailure(ctx, id)
}

// ClaimInvite atomically marks the named invite accepted, only if it
// hasn't already been accepted or revoked: the same
// "WHERE clause plus rows-affected check is the single race-decision
// point" shape ClaimPasswordResetToken already establishes.
func (db *DB) ClaimInvite(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE invites SET accepted_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("store: claim invite %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: claim invite %q: rows affected: %w", id, err)
	}
	if n > 0 {
		return nil
	}
	return db.classifyInviteMutationFailure(ctx, id)
}

func (db *DB) classifyInviteMutationFailure(ctx context.Context, id string) error {
	inv, err := db.GetInviteByID(ctx, id)
	if errors.Is(err, ErrInviteNotFound) {
		return ErrInviteNotFound
	}
	if err != nil {
		return err
	}
	if inv.AcceptedAt != nil {
		return ErrInviteAlreadyAccepted
	}
	return ErrInviteAlreadyRevoked
}
