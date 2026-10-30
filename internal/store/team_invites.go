package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTeamInviteInvalid covers an unknown, expired, revoked or already used
// invite, deliberately one error so a probe learns nothing about which.
var ErrTeamInviteInvalid = errors.New("store: team invite is invalid or expired")

// TeamInvite is a pending, accepted or revoked invitation into a team.
type TeamInvite struct {
	ID         string
	TeamID     string
	Email      string
	Role       string
	TokenHash  string
	CreatedBy  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	AcceptedBy string
	RevokedAt  *time.Time
}

const teamInviteColumns = `id, team_id, email, role, token_hash, created_by, created_at, expires_at, accepted_at, COALESCE(accepted_by, ''), revoked_at`

func scanTeamInvite(scan func(dest ...any) error) (TeamInvite, error) {
	var inv TeamInvite
	var created, expires string
	var accepted, revoked sql.NullString
	if err := scan(&inv.ID, &inv.TeamID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.CreatedBy, &created, &expires, &accepted, &inv.AcceptedBy, &revoked); err != nil {
		return TeamInvite{}, err
	}
	var err error
	if inv.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return TeamInvite{}, fmt.Errorf("parse created_at: %w", err)
	}
	if inv.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires); err != nil {
		return TeamInvite{}, fmt.Errorf("parse expires_at: %w", err)
	}
	if inv.AcceptedAt, err = parseTimePtr(accepted); err != nil {
		return TeamInvite{}, fmt.Errorf("parse accepted_at: %w", err)
	}
	if inv.RevokedAt, err = parseTimePtr(revoked); err != nil {
		return TeamInvite{}, fmt.Errorf("parse revoked_at: %w", err)
	}
	return inv, nil
}

// CreateTeamInvite stores a new invite. Only the token hash is kept.
func (db *DB) CreateTeamInvite(ctx context.Context, inv TeamInvite) error {
	if inv.Role == TeamRoleOwner || !ValidTeamRole(inv.Role) {
		return ErrInvalidTeamRole
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO team_invites (id, team_id, email, role, token_hash, created_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.TeamID, inv.Email, inv.Role, inv.TokenHash, inv.CreatedBy, formatTime(inv.CreatedAt), formatTime(inv.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store: create team invite: %w", err)
	}
	return nil
}

// ListTeamInvites returns a team's invites, newest first.
func (db *DB) ListTeamInvites(ctx context.Context, teamID string) ([]TeamInvite, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+teamInviteColumns+` FROM team_invites WHERE team_id = ? ORDER BY created_at DESC`, teamID)
	if err != nil {
		return nil, fmt.Errorf("store: list team invites: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TeamInvite
	for rows.Next() {
		inv, err := scanTeamInvite(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan team invite: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// GetTeamInviteByHash returns the invite for a token hash, whatever its state.
func (db *DB) GetTeamInviteByHash(ctx context.Context, tokenHash string) (TeamInvite, error) {
	inv, err := scanTeamInvite(db.QueryRowContext(ctx, `SELECT `+teamInviteColumns+` FROM team_invites WHERE token_hash = ?`, tokenHash).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamInvite{}, ErrTeamInviteInvalid
	}
	if err != nil {
		return TeamInvite{}, fmt.Errorf("store: get team invite: %w", err)
	}
	return inv, nil
}

// RevokeTeamInvite revokes a pending invite of this team.
func (db *DB) RevokeTeamInvite(ctx context.Context, teamID, id string, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE team_invites SET revoked_at = ?
		WHERE id = ? AND team_id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, formatTime(now), id, teamID)
	if err != nil {
		return fmt.Errorf("store: revoke team invite: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTeamInviteInvalid
	}
	return nil
}

// ClaimTeamInvite marks a usable invite accepted by userID, at most once.
// The conditional UPDATE is what stops a replay racing a first use.
func (db *DB) ClaimTeamInvite(ctx context.Context, tokenHash, userID string, now time.Time) (TeamInvite, error) {
	inv, err := db.GetTeamInviteByHash(ctx, tokenHash)
	if err != nil {
		return TeamInvite{}, err
	}
	if inv.AcceptedAt != nil || inv.RevokedAt != nil || !now.Before(inv.ExpiresAt) {
		return TeamInvite{}, ErrTeamInviteInvalid
	}
	res, err := db.ExecContext(ctx, `
		UPDATE team_invites SET accepted_at = ?, accepted_by = ?
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL`, formatTime(now), userID, inv.ID)
	if err != nil {
		return TeamInvite{}, fmt.Errorf("store: claim team invite: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return TeamInvite{}, ErrTeamInviteInvalid
	}
	inv.AcceptedAt = &now
	inv.AcceptedBy = userID
	return inv, nil
}

// APITokenScope limits a token to one team, optionally one project, and a role.
// Stack, when set, also binds it to one stack; it is stored separately.
type APITokenScope struct {
	TokenID   string
	TeamID    string
	ProjectID string
	Role      string
	Stack     *APITokenStackScope
}

// SetAPITokenScope records a token's team scope.
func (db *DB) SetAPITokenScope(ctx context.Context, s APITokenScope) error {
	if !ValidTeamRole(s.Role) || s.TeamID == "" {
		return ErrInvalidTeamRole
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO api_token_scopes (token_id, team_id, project_id, role) VALUES (?, ?, ?, ?)
		ON CONFLICT(token_id) DO UPDATE SET team_id = excluded.team_id, project_id = excluded.project_id, role = excluded.role`,
		s.TokenID, s.TeamID, s.ProjectID, s.Role)
	if err != nil {
		return fmt.Errorf("store: set token scope: %w", err)
	}
	return nil
}

// GetAPITokenScope returns a token's scope, or nil when it is unscoped.
func (db *DB) GetAPITokenScope(ctx context.Context, tokenID string) (*APITokenScope, error) {
	s := APITokenScope{TokenID: tokenID}
	err := db.QueryRowContext(ctx, `SELECT team_id, project_id, role FROM api_token_scopes WHERE token_id = ?`, tokenID).
		Scan(&s.TeamID, &s.ProjectID, &s.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get token scope: %w", err)
	}
	return &s, nil
}

// Stack token verbs. Destroy, unlock and settings are deliberately absent:
// those stay with a human admin.
const (
	StackVerbPlan  = "plan"
	StackVerbApply = "apply"
)

// ErrInvalidStackScope rejects a stack scope without a stack or with an unknown verb.
var ErrInvalidStackScope = errors.New("store: invalid token stack scope")

// APITokenStackScope binds a token to one stack and the verbs it may use there.
type APITokenStackScope struct {
	TokenID string
	StackID string
	Verbs   []string
}

// Has reports whether the scope grants verb.
func (s APITokenStackScope) Has(verb string) bool {
	for _, v := range s.Verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// ValidStackVerbs reports whether verbs is a non-empty set of plan and apply.
func ValidStackVerbs(verbs []string) bool {
	if len(verbs) == 0 {
		return false
	}
	for _, v := range verbs {
		if v != StackVerbPlan && v != StackVerbApply {
			return false
		}
	}
	return true
}

// SetAPITokenStackScope records a token's stack scope.
func (db *DB) SetAPITokenStackScope(ctx context.Context, s APITokenStackScope) error {
	if s.TokenID == "" || s.StackID == "" || !ValidStackVerbs(s.Verbs) {
		return ErrInvalidStackScope
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO api_token_stack_scopes (token_id, stack_id, verbs) VALUES (?, ?, ?)
		ON CONFLICT(token_id) DO UPDATE SET stack_id = excluded.stack_id, verbs = excluded.verbs`,
		s.TokenID, s.StackID, strings.Join(s.Verbs, ","))
	if err != nil {
		return fmt.Errorf("store: set token stack scope: %w", err)
	}
	return nil
}

// GetAPITokenStackScope returns a token's stack scope, or nil when it has none.
// A stored row that no longer parses is an error, so callers fail closed.
func (db *DB) GetAPITokenStackScope(ctx context.Context, tokenID string) (*APITokenStackScope, error) {
	s := APITokenStackScope{TokenID: tokenID}
	var verbs string
	err := db.QueryRowContext(ctx, `SELECT stack_id, verbs FROM api_token_stack_scopes WHERE token_id = ?`, tokenID).Scan(&s.StackID, &verbs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get token stack scope: %w", err)
	}
	s.Verbs = strings.Split(verbs, ",")
	if s.StackID == "" || !ValidStackVerbs(s.Verbs) {
		return nil, ErrInvalidStackScope
	}
	return &s, nil
}

// TeamEvent is one membership change in a team's own audit trail.
type TeamEvent struct {
	ID        string
	TeamID    string
	ActorID   string
	ActorName string
	Action    string
	Subject   string
	Detail    string
	CreatedAt time.Time
}

// RecordTeamEvent appends to a team's audit trail.
func (db *DB) RecordTeamEvent(ctx context.Context, e TeamEvent) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO team_events (id, team_id, actor_id, actor_name, action, subject, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, e.ID, e.TeamID, e.ActorID, e.ActorName, e.Action, e.Subject, e.Detail, formatTime(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: record team event: %w", err)
	}
	return nil
}

// ListTeamEvents returns a team's newest events, at most limit.
func (db *DB) ListTeamEvents(ctx context.Context, teamID string, limit int) ([]TeamEvent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, team_id, actor_id, actor_name, action, subject, detail, created_at
		FROM team_events WHERE team_id = ? ORDER BY created_at DESC LIMIT ?`, teamID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list team events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TeamEvent
	for rows.Next() {
		var e TeamEvent
		var created string
		if err := rows.Scan(&e.ID, &e.TeamID, &e.ActorID, &e.ActorName, &e.Action, &e.Subject, &e.Detail, &created); err != nil {
			return nil, fmt.Errorf("store: scan team event: %w", err)
		}
		if e.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse team event time: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
