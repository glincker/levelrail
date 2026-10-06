package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// CountRootUsersExcept counts users holding the root ability, ignoring one user.
func (db *DB) CountRootUsersExcept(ctx context.Context, userID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE id != ? AND EXISTS (SELECT 1 FROM json_each(users.abilities) WHERE value = 'root')`,
		userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count root users: %w", err)
	}
	return n, nil
}

// CountRootUsersOutsideRole counts root users who do not hold the given role.
func (db *DB) CountRootUsersOutsideRole(ctx context.Context, roleID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE COALESCE(role_id, '') != ? AND EXISTS (SELECT 1 FROM json_each(users.abilities) WHERE value = 'root')`,
		roleID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count root users outside role: %w", err)
	}
	return n, nil
}

// SyncRoleAbilities rewrites the abilities of every user holding the role to the role's current set.
func (db *DB) SyncRoleAbilities(ctx context.Context, roleID string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET abilities = (SELECT abilities FROM roles WHERE id = ?) WHERE role_id = ?`, roleID, roleID)
	if err != nil {
		return fmt.Errorf("store: sync role abilities %q: %w", roleID, err)
	}
	return nil
}

// UpdateRoleAndSync rewrites a custom role and the abilities of every user
// holding it in one transaction, so a downgraded role can never leave its
// users with the old, higher abilities.
func (db *DB) UpdateRoleAndSync(ctx context.Context, r Role) error {
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
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: update role %q: %w", r.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`UPDATE roles SET name = ?, description = ?, abilities = ?, visibility = ? WHERE id = ?`,
		r.Name, r.Description, string(abilities), r.Visibility, r.ID); err != nil {
		if isUniqueViolation(err) {
			return ErrRoleNameTaken
		}
		return fmt.Errorf("store: update role %q: %w", r.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET abilities = ? WHERE role_id = ?`, string(abilities), r.ID); err != nil {
		return fmt.Errorf("store: sync role abilities %q: %w", r.ID, err)
	}
	return tx.Commit()
}
