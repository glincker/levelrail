package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Role visibility: a role either sees every resource its abilities allow, or
// only resources in the environments granted to each user holding it.
const (
	RoleVisibilityAll     = "all"
	RoleVisibilityGranted = "granted"
)

var (
	// ErrRoleNotFound is returned when no role has the given ID.
	ErrRoleNotFound = errors.New("store: role not found")
	// ErrRoleNameTaken is returned when a role name is already in use.
	ErrRoleNameTaken = errors.New("store: a role with that name already exists")
	// ErrRoleBuiltin is returned when a built-in role would be edited or deleted.
	ErrRoleBuiltin = errors.New("store: built-in roles cannot be changed")
	// ErrRoleInUse is returned when a role still has users.
	ErrRoleInUse = errors.New("store: the role is assigned to users")
)

// Role is a named, stored bundle of abilities (migration 0374).
type Role struct {
	ID          string
	Name        string
	Description string
	Abilities   []string
	Visibility  string
	Builtin     bool
	CreatedAt   string
}

const roleColumns = `id, name, description, abilities, visibility, builtin, created_at`

func scanRole(scan func(dest ...any) error) (Role, error) {
	var r Role
	var abilities string
	var builtin int
	if err := scan(&r.ID, &r.Name, &r.Description, &abilities, &r.Visibility, &builtin, &r.CreatedAt); err != nil {
		return Role{}, err
	}
	if err := json.Unmarshal([]byte(abilities), &r.Abilities); err != nil {
		return Role{}, fmt.Errorf("unmarshal role abilities: %w", err)
	}
	r.Builtin = builtin == 1
	return r, nil
}

// ListRoles returns built-in roles first, then custom ones by name.
func (db *DB) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+roleColumns+` FROM roles ORDER BY builtin DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("store: list roles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Role
	for rows.Next() {
		r, err := scanRole(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: list roles: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRole returns the role with this ID, or ErrRoleNotFound.
func (db *DB) GetRole(ctx context.Context, id string) (Role, error) {
	r, err := scanRole(db.QueryRowContext(ctx, `SELECT `+roleColumns+` FROM roles WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Role{}, ErrRoleNotFound
	}
	if err != nil {
		return Role{}, fmt.Errorf("store: get role %q: %w", id, err)
	}
	return r, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// CreateRole inserts a custom role. Callers validate the abilities.
func (db *DB) CreateRole(ctx context.Context, r Role) error {
	abilities, err := json.Marshal(nonNilSlice(r.Abilities))
	if err != nil {
		return fmt.Errorf("store: create role: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO roles (id, name, description, abilities, visibility, builtin) VALUES (?, ?, ?, ?, ?, 0)`,
		r.ID, r.Name, r.Description, string(abilities), r.Visibility)
	if isUniqueViolation(err) {
		return ErrRoleNameTaken
	}
	if err != nil {
		return fmt.Errorf("store: create role %q: %w", r.Name, err)
	}
	return nil
}

// UpdateRole rewrites a custom role. A built-in role returns ErrRoleBuiltin.
func (db *DB) UpdateRole(ctx context.Context, r Role) error {
	cur, err := db.GetRole(ctx, r.ID)
	if err != nil {
		return err
	}
	if cur.Builtin {
		return ErrRoleBuiltin
	}
	abilities, err := json.Marshal(nonNilSlice(r.Abilities))
	if err != nil {
		return fmt.Errorf("store: update role: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`UPDATE roles SET name = ?, description = ?, abilities = ?, visibility = ? WHERE id = ?`,
		r.Name, r.Description, string(abilities), r.Visibility, r.ID)
	if isUniqueViolation(err) {
		return ErrRoleNameTaken
	}
	if err != nil {
		return fmt.Errorf("store: update role %q: %w", r.ID, err)
	}
	return nil
}

// DeleteRole removes a custom role that has no users.
func (db *DB) DeleteRole(ctx context.Context, id string) error {
	cur, err := db.GetRole(ctx, id)
	if err != nil {
		return err
	}
	if cur.Builtin {
		return ErrRoleBuiltin
	}
	counts, err := db.RoleUserCounts(ctx)
	if err != nil {
		return err
	}
	if counts[id] > 0 {
		return ErrRoleInUse
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete role %q: %w", id, err)
	}
	return nil
}

// RoleUserCounts maps role ID to how many users hold it.
func (db *DB) RoleUserCounts(ctx context.Context) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT role_id, COUNT(*) FROM users WHERE role_id IS NOT NULL GROUP BY role_id`)
	if err != nil {
		return nil, fmt.Errorf("store: count role users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store: count role users: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// UserRoleID returns the role assigned to a user, or "" when none is.
func (db *DB) UserRoleID(ctx context.Context, userID string) (string, error) {
	var id sql.NullString
	err := db.QueryRowContext(ctx, `SELECT role_id FROM users WHERE id = ?`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: user role %q: %w", userID, err)
	}
	return id.String, nil
}

// SetUserRole assigns a role and copies its abilities onto the user in one
// transaction, so the abilities the gate enforces never drift from the role.
func (db *DB) SetUserRole(ctx context.Context, userID, roleID string) error {
	role, err := db.GetRole(ctx, roleID)
	if err != nil {
		return err
	}
	abilities, err := json.Marshal(nonNilSlice(role.Abilities))
	if err != nil {
		return fmt.Errorf("store: set user role: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE users SET role_id = ?, abilities = ? WHERE id = ?`, roleID, string(abilities), userID)
	if err != nil {
		return fmt.Errorf("store: set user role %q: %w", userID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ListUserEnvironmentGrants returns the environment IDs granted to a user.
func (db *DB) ListUserEnvironmentGrants(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT environment_id FROM user_environment_grants WHERE user_id = ? ORDER BY environment_id`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: list grants: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetUserEnvironmentGrants replaces a user's grants atomically.
func (db *DB) SetUserEnvironmentGrants(ctx context.Context, userID string, environmentIDs []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set grants: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_environment_grants WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: set grants: %w", err)
	}
	for _, id := range environmentIDs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO user_environment_grants (user_id, environment_id) VALUES (?, ?)`, userID, id); err != nil {
			return fmt.Errorf("store: grant %q to %q: %w", id, userID, err)
		}
	}
	return tx.Commit()
}

// Visibility says what a user may see: everything their abilities allow, or
// only the listed environments.
type Visibility struct {
	Restricted     bool
	EnvironmentIDs map[string]bool
}

// UserVisibility is restricted when the user's role has granted visibility.
// A user with no role, or a role that sees everything, is unrestricted.
func (db *DB) UserVisibility(ctx context.Context, userID string) (Visibility, error) {
	var vis sql.NullString
	err := db.QueryRowContext(ctx, `SELECT r.visibility FROM users u LEFT JOIN roles r ON r.id = u.role_id WHERE u.id = ?`, userID).Scan(&vis)
	if errors.Is(err, sql.ErrNoRows) {
		return Visibility{}, ErrUserNotFound
	}
	if err != nil {
		return Visibility{}, fmt.Errorf("store: user visibility %q: %w", userID, err)
	}
	if vis.String != RoleVisibilityGranted {
		return Visibility{}, nil
	}
	ids, err := db.ListUserEnvironmentGrants(ctx, userID)
	if err != nil {
		return Visibility{}, err
	}
	granted := make(map[string]bool, len(ids))
	for _, id := range ids {
		granted[id] = true
	}
	return Visibility{Restricted: true, EnvironmentIDs: granted}, nil
}

// Allows reports whether a resource in this environment is visible. A resource
// with no environment is visible only to unrestricted users.
func (v Visibility) Allows(environmentID string) bool {
	if !v.Restricted {
		return true
	}
	return environmentID != "" && v.EnvironmentIDs[environmentID]
}
