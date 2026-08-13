package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AdminUser is the single operator account TASKS.md 1.9 and CLAUDE.md 6
// Phase 1 call for: "Single admin user, session auth. No teams, no RBAC
// yet." There is exactly one row in the admin_user table, ever, enforced
// by that table's CHECK (id = 1) constraint, not by application logic.
type AdminUser struct {
	Username     string
	PasswordHash string
}

// ErrAdminNotFound is returned by GetAdminUser before the admin account
// has been bootstrapped.
var ErrAdminNotFound = errors.New("store: admin user not found")

// GetAdminUser returns the single admin account, or ErrAdminNotFound if
// none has been created yet.
func (db *DB) GetAdminUser(ctx context.Context) (*AdminUser, error) {
	var u AdminUser
	err := db.QueryRowContext(ctx, `
		SELECT username, password_hash FROM admin_user WHERE id = 1
	`).Scan(&u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAdminNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get admin user: %w", err)
	}
	return &u, nil
}

// UpsertAdminUser creates or replaces the single admin account. Called at
// bootstrap (no admin exists yet) and whenever the password changes;
// both are "replace the one row" from this table's point of view, same
// upsert shape as SaveDesiredService.
func (db *DB) UpsertAdminUser(ctx context.Context, username, passwordHash string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO admin_user (id, username, password_hash, updated_at)
		VALUES (1, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (id) DO UPDATE SET
			username = excluded.username,
			password_hash = excluded.password_hash,
			updated_at = excluded.updated_at
	`, username, passwordHash)
	if err != nil {
		return fmt.Errorf("store: upsert admin user: %w", err)
	}
	return nil
}

// ErrAdminAlreadyExists is CreateAdminUser's failure mode when a row
// already exists: unlike UpsertAdminUser, this never overwrites.
var ErrAdminAlreadyExists = errors.New("store: admin user already exists")

// CreateAdminUser inserts the single admin account, failing with
// ErrAdminAlreadyExists if one already exists, rather than silently
// replacing it the way UpsertAdminUser does. This is what
// internal/api's first-run registration handler needs: a caller-side
// "does an admin exist yet" check alone can't close the race between
// two concurrent registration attempts (both can read "not found"
// before either writes), but a plain INSERT with no ON CONFLICT clause
// lets admin_user's own PRIMARY KEY constraint (id = 1) decide, which is
// atomic at the database level regardless of how many goroutines race
// to call this. On any INSERT failure, this re-checks existence to
// distinguish "lost the race" (ErrAdminAlreadyExists) from a genuine,
// unrelated database error, rather than assuming every failure means
// the former.
func (db *DB) CreateAdminUser(ctx context.Context, username, passwordHash string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO admin_user (id, username, password_hash, updated_at)
		VALUES (1, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	`, username, passwordHash)
	if err == nil {
		return nil
	}

	if _, getErr := db.GetAdminUser(ctx); getErr == nil {
		return ErrAdminAlreadyExists
	}
	return fmt.Errorf("store: create admin user: %w", err)
}
