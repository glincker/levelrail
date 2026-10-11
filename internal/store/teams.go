package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// DefaultTeamID is the team migration 0442 creates to own everything that
// existed before teams. Its members are the instance's own users.
const DefaultTeamID = "org_default"

// Team roles, ordered from most to least privileged.
const (
	TeamRoleOwner     = "owner"
	TeamRoleAdmin     = "admin"
	TeamRoleDeveloper = "developer"
	TeamRoleViewer    = "viewer"
)

var (
	// ErrTeamMemberNotFound is returned when the user is not in the team.
	ErrTeamMemberNotFound = errors.New("store: team member not found")
	// ErrTeamMemberExists is returned when the user is already in the team.
	ErrTeamMemberExists = errors.New("store: user is already a member of the team")
	// ErrLastTeamOwner is returned when a change would leave a team with no owner.
	ErrLastTeamOwner = errors.New("store: a team must keep at least one owner")
	// ErrInvalidTeamRole is returned for a role outside the fixed set.
	ErrInvalidTeamRole = errors.New("store: invalid team role")
)

var teamRoleRank = map[string]int{
	TeamRoleOwner:     4,
	TeamRoleAdmin:     3,
	TeamRoleDeveloper: 2,
	TeamRoleViewer:    1,
}

// ValidTeamRole reports whether role is one of the four team roles.
func ValidTeamRole(role string) bool {
	_, ok := teamRoleRank[role]
	return ok
}

// TeamRoleAtLeast reports whether role ranks at or above min. Unknown roles
// rank below everything.
func TeamRoleAtLeast(role, minimum string) bool {
	return teamRoleRank[role] >= teamRoleRank[minimum] && teamRoleRank[role] > 0
}

// LowerTeamRole returns the less privileged of two roles.
func LowerTeamRole(a, b string) string {
	if teamRoleRank[a] <= teamRoleRank[b] {
		return a
	}
	return b
}

// TeamRoleAbilities maps a team role onto the flat ability model, so team
// roles and IAM policies share one evaluation path.
func TeamRoleAbilities(role string) []string {
	switch role {
	case TeamRoleOwner, TeamRoleAdmin, TeamRoleDeveloper:
		return []string{"deploy", "read", "read:sensitive", "write", "write:sensitive"}
	case TeamRoleViewer:
		return []string{"read"}
	}
	return nil
}

// TeamMember is one user's membership of a team.
type TeamMember struct {
	TeamID      string
	UserID      string
	Email       string
	DisplayName string
	Role        string
	CreatedAt   string
}

// ListTeamMembers returns a team's members, owners first.
func (db *DB) ListTeamMembers(ctx context.Context, teamID string) ([]TeamMember, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT m.team_id, m.user_id, u.email, u.display_name, m.role, m.created_at
		FROM team_members m JOIN users u ON u.id = m.user_id
		WHERE m.team_id = ?
		ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 WHEN 'developer' THEN 2 ELSE 3 END, u.email`, teamID)
	if err != nil {
		return nil, fmt.Errorf("store: list team members %q: %w", teamID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.TeamID, &m.UserID, &m.Email, &m.DisplayName, &m.Role, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan team member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UserTeamRoles maps each team the user belongs to onto their role there.
func (db *DB) UserTeamRoles(ctx context.Context, userID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT team_id, role FROM team_members WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: user team roles %q: %w", userID, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var team, role string
		if err := rows.Scan(&team, &role); err != nil {
			return nil, fmt.Errorf("store: scan team role: %w", err)
		}
		out[team] = role
	}
	return out, rows.Err()
}

// TeamMemberRole returns the user's role in the team, or ErrTeamMemberNotFound.
func (db *DB) TeamMemberRole(ctx context.Context, teamID, userID string) (string, error) {
	var role string
	err := db.QueryRowContext(ctx, `SELECT role FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTeamMemberNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: team member role: %w", err)
	}
	return role, nil
}

// AddTeamMember inserts a membership.
func (db *DB) AddTeamMember(ctx context.Context, teamID, userID, role string) error {
	if !ValidTeamRole(role) {
		return ErrInvalidTeamRole
	}
	if _, err := db.TeamMemberRole(ctx, teamID, userID); err == nil {
		return ErrTeamMemberExists
	}
	_, err := db.ExecContext(ctx, `INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		teamID, userID, role, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: add team member: %w", err)
	}
	return db.syncAbilitiesFromRoles(ctx, userID, false)
}

// SetTeamMemberRole changes a member's role, refusing to demote the last owner.
func (db *DB) SetTeamMemberRole(ctx context.Context, teamID, userID, role string) error {
	if !ValidTeamRole(role) {
		return ErrInvalidTeamRole
	}
	err := db.withTeamTx(ctx, func(tx *sql.Tx) error {
		current, err := txMemberRole(ctx, tx, teamID, userID)
		if err != nil {
			return err
		}
		if current == TeamRoleOwner && role != TeamRoleOwner {
			if err := txRequireAnotherOwner(ctx, tx, teamID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?`, role, teamID, userID)
		return err
	})
	if err != nil {
		return err
	}
	return db.syncAbilitiesFromRoles(ctx, userID, true)
}

// RemoveTeamMember deletes a membership, refusing to remove the last owner.
func (db *DB) RemoveTeamMember(ctx context.Context, teamID, userID string) error {
	err := db.withTeamTx(ctx, func(tx *sql.Tx) error {
		current, err := txMemberRole(ctx, tx, teamID, userID)
		if err != nil {
			return err
		}
		if current == TeamRoleOwner {
			if err := txRequireAnotherOwner(ctx, tx, teamID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID)
		return err
	})
	if err != nil {
		return err
	}
	return db.syncAbilitiesFromRoles(ctx, userID, false)
}

// TransferTeamOwnership makes to an owner and from an admin, atomically.
func (db *DB) TransferTeamOwnership(ctx context.Context, teamID, from, to string) error {
	err := db.withTeamTx(ctx, func(tx *sql.Tx) error {
		fromRole, err := txMemberRole(ctx, tx, teamID, from)
		if err != nil {
			return err
		}
		if fromRole != TeamRoleOwner {
			return ErrTeamMemberNotFound
		}
		if _, err := txMemberRole(ctx, tx, teamID, to); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE team_members SET role = 'owner' WHERE team_id = ? AND user_id = ?`, teamID, to); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE team_members SET role = 'admin' WHERE team_id = ? AND user_id = ?`, teamID, from)
		return err
	})
	if err != nil {
		return err
	}
	if err := db.syncAbilitiesFromRoles(ctx, from, true); err != nil {
		return err
	}
	return db.syncAbilitiesFromRoles(ctx, to, true)
}

func (db *DB) withTeamTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin team tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		if errors.Is(err, ErrTeamMemberNotFound) || errors.Is(err, ErrLastTeamOwner) {
			return err
		}
		return fmt.Errorf("store: team membership change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit team tx: %w", err)
	}
	return nil
}

func txMemberRole(ctx context.Context, tx *sql.Tx, teamID, userID string) (string, error) {
	var role string
	err := tx.QueryRowContext(ctx, `SELECT role FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTeamMemberNotFound
	}
	return role, err
}

func txRequireAnotherOwner(ctx context.Context, tx *sql.Tx, teamID string) error {
	var owners int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM team_members WHERE team_id = ? AND role = 'owner'`, teamID).Scan(&owners); err != nil {
		return err
	}
	if owners <= 1 {
		return ErrLastTeamOwner
	}
	return nil
}

// DefaultTeamRole picks the default team role matching flat abilities.
func DefaultTeamRole(abilities []string) string {
	switch {
	case slices.Contains(abilities, "root"):
		return TeamRoleOwner
	case slices.Contains(abilities, "deploy"):
		return TeamRoleAdmin
	case slices.Contains(abilities, "write"):
		return TeamRoleDeveloper
	}
	return TeamRoleViewer
}

func (db *DB) joinInitialTeam(ctx context.Context, u User) error {
	team, role := u.TeamID, u.TeamRole
	if team == "" {
		team = DefaultTeamID
	}
	if role == "" {
		role = DefaultTeamRole(u.Abilities)
	}
	if err := db.AddTeamMember(ctx, team, u.ID, role); err != nil {
		return fmt.Errorf("store: create user %q: join team: %w", u.ID, err)
	}
	return nil
}

// syncAbilitiesFromRoles sets a user's flat abilities to the union of their
// team roles, so route level checks never exceed what teams granted. Default
// team members keep hand-set abilities unless a role was changed explicitly;
// root users are never touched.
func (db *DB) syncAbilitiesFromRoles(ctx context.Context, userID string, explicit bool) error {
	user, err := db.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("store: sync abilities: %w", err)
	}
	if slices.Contains(user.Abilities, "root") {
		return nil
	}
	roles, err := db.UserTeamRoles(ctx, userID)
	if err != nil {
		return err
	}
	if _, instance := roles[DefaultTeamID]; instance && !explicit {
		return nil
	}
	var union []string
	for _, role := range roles {
		for _, a := range TeamRoleAbilities(role) {
			if !slices.Contains(union, a) {
				union = append(union, a)
			}
		}
	}
	slices.Sort(union)
	return db.UpdateUserAbilities(ctx, userID, union)
}
