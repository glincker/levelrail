package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSSOGroupDuplicate is returned when one group appears twice in a mapping set.
var ErrSSOGroupDuplicate = errors.New("store: group is mapped more than once")

// SSOGroupMapping maps an IdP or SCIM group name to a stored role.
type SSOGroupMapping struct {
	Group    string
	RoleID   string
	Priority int
}

// SSOSettings is the singleton single sign-on settings row (migration 0751).
type SSOSettings struct {
	EnforceSSO        bool
	SCIMDefaultRoleID string
}

// ListSSOGroupMappings returns mappings, highest priority first.
func (db *DB) ListSSOGroupMappings(ctx context.Context) ([]SSOGroupMapping, error) {
	rows, err := db.QueryContext(ctx, `SELECT group_name, role_id, priority FROM sso_group_role_mappings ORDER BY priority DESC, group_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list sso group mappings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SSOGroupMapping
	for rows.Next() {
		var m SSOGroupMapping
		if err := rows.Scan(&m.Group, &m.RoleID, &m.Priority); err != nil {
			return nil, fmt.Errorf("store: scan sso group mapping: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list sso group mappings: %w", err)
	}
	return out, nil
}

// ReplaceSSOGroupMappings swaps the whole mapping set in one transaction.
// Every role must exist; an unknown role fails the call with ErrRoleNotFound.
func (db *DB) ReplaceSSOGroupMappings(ctx context.Context, mappings []SSOGroupMapping, now time.Time) error {
	seen := map[string]bool{}
	for _, m := range mappings {
		key := strings.ToLower(strings.TrimSpace(m.Group))
		if seen[key] {
			return ErrSSOGroupDuplicate
		}
		seen[key] = true
		if _, err := db.GetRole(ctx, m.RoleID); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace sso group mappings: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sso_group_role_mappings`); err != nil {
		return fmt.Errorf("store: replace sso group mappings: %w", err)
	}
	for _, m := range mappings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sso_group_role_mappings (group_name, role_id, priority, created_at) VALUES (?, ?, ?, ?)`,
			strings.TrimSpace(m.Group), m.RoleID, m.Priority, formatTime(now)); err != nil {
			return fmt.Errorf("store: replace sso group mappings: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace sso group mappings: %w", err)
	}
	return nil
}

// GetSSOSettings reads the singleton settings row.
func (db *DB) GetSSOSettings(ctx context.Context) (SSOSettings, error) {
	var s SSOSettings
	var enforce int
	err := db.QueryRowContext(ctx, `SELECT enforce_sso, scim_default_role_id FROM sso_settings WHERE id = 1`).Scan(&enforce, &s.SCIMDefaultRoleID)
	if err != nil {
		return SSOSettings{}, fmt.Errorf("store: get sso settings: %w", err)
	}
	s.EnforceSSO = enforce == 1
	return s, nil
}

// SetSSOEnforce turns SSO enforcement on or off.
func (db *DB) SetSSOEnforce(ctx context.Context, on bool, now time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE sso_settings SET enforce_sso = ?, updated_at = ? WHERE id = 1`, boolInt(on), formatTime(now)); err != nil {
		return fmt.Errorf("store: set sso enforce: %w", err)
	}
	return nil
}

// SetSCIMDefaultRole sets the role given to directory users no group mapping matches.
func (db *DB) SetSCIMDefaultRole(ctx context.Context, roleID string, now time.Time) error {
	if _, err := db.GetRole(ctx, roleID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `UPDATE sso_settings SET scim_default_role_id = ?, updated_at = ? WHERE id = 1`, roleID, formatTime(now)); err != nil {
		return fmt.Errorf("store: set scim default role: %w", err)
	}
	return nil
}

// SCIMTokenMeta is the display data kept beside a library SCIM token.
type SCIMTokenMeta struct {
	Prefix    string
	CreatedBy string
}

// SaveSCIMTokenMeta records display data for a library token.
func (db *DB) SaveSCIMTokenMeta(ctx context.Context, tokenID string, m SCIMTokenMeta) error {
	if _, err := db.ExecContext(ctx, `INSERT INTO sso_scim_token_meta (token_id, prefix, created_by) VALUES (?, ?, ?)
		ON CONFLICT (token_id) DO UPDATE SET prefix = excluded.prefix, created_by = excluded.created_by`, tokenID, m.Prefix, m.CreatedBy); err != nil {
		return fmt.Errorf("store: save scim token meta: %w", err)
	}
	return nil
}

// SCIMTokenMetas returns display data keyed by library token id.
func (db *DB) SCIMTokenMetas(ctx context.Context) (map[string]SCIMTokenMeta, error) {
	rows, err := db.QueryContext(ctx, `SELECT token_id, prefix, created_by FROM sso_scim_token_meta`)
	if err != nil {
		return nil, fmt.Errorf("store: list scim token meta: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]SCIMTokenMeta{}
	for rows.Next() {
		var id string
		var m SCIMTokenMeta
		if err := rows.Scan(&id, &m.Prefix, &m.CreatedBy); err != nil {
			return nil, fmt.Errorf("store: scan scim token meta: %w", err)
		}
		out[id] = m
	}
	return out, rows.Err()
}

// CreateProvisionedUser creates a passwordless user with roleID and puts it
// in the default team, all in one transaction.
func (db *DB) CreateProvisionedUser(ctx context.Context, u User, roleID string) error {
	role, err := db.GetRole(ctx, roleID)
	if err != nil {
		return err
	}
	abilities, err := json.Marshal(nonNilSlice(role.Abilities))
	if err != nil {
		return fmt.Errorf("store: provision user: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: provision user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `INSERT INTO users (id, email, display_name, password_hash, abilities, is_first_user, created_at, last_login_at, role_id)
		VALUES (?, ?, ?, NULL, ?, 0, ?, NULL, ?)`, u.ID, u.Email, u.DisplayName, string(abilities), formatTime(u.CreatedAt), role.ID)
	if isUniqueViolation(err) && strings.Contains(err.Error(), "email") {
		return ErrUserEmailExists
	}
	if err != nil {
		return fmt.Errorf("store: provision user %q: %w", u.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO team_members (team_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		DefaultTeamID, u.ID, DefaultTeamRole(role.Abilities), formatTime(u.CreatedAt)); err != nil {
		return fmt.Errorf("store: provision user %q: join team: %w", u.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: provision user %q: %w", u.ID, err)
	}
	return nil
}
