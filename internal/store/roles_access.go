package store

import (
	"context"
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
