package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User is a real, individually-identified account
// (migrations/0030_users.sql). Every user has identical access, no role
// field. PasswordHash is nil for an OAuth-only user. IsFirstUser is
// display-only.
type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash *string
	IsFirstUser  bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// ErrUserNotFound is returned by GetUserByID and GetUserByEmail when no
// row matches.
var ErrUserNotFound = errors.New("store: user not found")

// ErrUserEmailExists is CreateUser's failure mode when email is already
// taken by a different row.
var ErrUserEmailExists = errors.New("store: user email already exists")

// ErrFirstUserExists is CreateUser's failure mode when u.IsFirstUser is
// true but a first user already exists (ux_users_single_first_user).
var ErrFirstUserExists = errors.New("store: first user already exists")

// CreateUser inserts a new user row, insert-only, never an upsert. On
// failure it re-checks by the relevant unique constraint to classify the
// cause, rather than assuming every failure means the same thing.
func (db *DB) CreateUser(ctx context.Context, u User) error {
	isFirst := 0
	if u.IsFirstUser {
		isFirst = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO users (id, email, display_name, password_hash, is_first_user, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, u.ID, u.Email, u.DisplayName, nullableString(u.PasswordHash), isFirst, formatTime(u.CreatedAt), formatTimePtr(u.LastLoginAt))
	if err == nil {
		return nil
	}

	if u.IsFirstUser {
		if _, getErr := db.getFirstUser(ctx); getErr == nil {
			return ErrFirstUserExists
		}
	}
	if _, getErr := db.GetUserByEmail(ctx, u.Email); getErr == nil {
		return ErrUserEmailExists
	}
	return fmt.Errorf("store: create user %q: %w", u.ID, err)
}

// GetUserByID returns the user with this ID, or ErrUserNotFound.
func (db *DB) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, email, display_name, password_hash, is_first_user, created_at, last_login_at
		FROM users WHERE id = ?
	`, id)
	return scanUser(row.Scan)
}

// GetUserByEmail returns the user with this email, or ErrUserNotFound.
// The stored value isn't always a syntactically valid email: a
// pre-migration bare username migrates in unchanged (migrations/0030).
func (db *DB) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, email, display_name, password_hash, is_first_user, created_at, last_login_at
		FROM users WHERE email = ?
	`, email)
	return scanUser(row.Scan)
}

func (db *DB) getFirstUser(ctx context.Context) (*User, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, email, display_name, password_hash, is_first_user, created_at, last_login_at
		FROM users WHERE is_first_user = 1
	`)
	return scanUser(row.Scan)
}

// ListUsers returns every user, oldest first.
func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, email, display_name, password_hash, is_first_user, created_at, last_login_at
		FROM users ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan user row: %w", err)
		}
		out = append(out, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate user rows: %w", err)
	}
	return out, nil
}

// CountUsers returns how many user rows exist, BootstrapAdmin's "has
// anyone signed up yet" check.
func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count users: %w", err)
	}
	return n, nil
}

// UpdateUserPasswordHash sets (or, given a nil hash, clears) a user's
// password. Returns ErrUserNotFound if id doesn't exist.
func (db *DB) UpdateUserPasswordHash(ctx context.Context, id string, hash *string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE users SET password_hash = ? WHERE id = ?
	`, nullableString(hash), id)
	if err != nil {
		return fmt.Errorf("store: update user %q password: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrUserNotFound, "update user %q password", id)
}

// UpdateUserLastLogin stamps last_login_at, called once per successful
// sign-in (password or OAuth). Returns ErrUserNotFound if id doesn't
// exist.
func (db *DB) UpdateUserLastLogin(ctx context.Context, id string, when time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE users SET last_login_at = ? WHERE id = ?
	`, formatTime(when), id)
	if err != nil {
		return fmt.Errorf("store: update user %q last login: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrUserNotFound, "update user %q last login", id)
}

// DeleteUser removes a user row; the FK cascade (migrations/0030) takes
// any linked OAuth identities with it. Guards like "not the last user"
// live in internal/api/users.go, not here.
func (db *DB) DeleteUser(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete user %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrUserNotFound, "delete user %q", id)
}

func scanUser(scan func(dest ...any) error) (*User, error) {
	var (
		u             User
		passwordHash  sql.NullString
		isFirst       int
		createdAt     string
		lastLoginNull sql.NullString
	)
	if err := scan(&u.ID, &u.Email, &u.DisplayName, &passwordHash, &isFirst, &createdAt, &lastLoginNull); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	if passwordHash.Valid {
		u.PasswordHash = &passwordHash.String
	}
	u.IsFirstUser = isFirst != 0
	parsedCreated, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	u.CreatedAt = parsedCreated
	u.LastLoginAt, err = parseTimePtr(lastLoginNull)
	if err != nil {
		return nil, fmt.Errorf("parse last_login_at: %w", err)
	}
	return &u, nil
}

func nullableString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// rowsAffectedOrNotFound translates "zero rows changed" into notFound,
// the shared tail of a conditional single-row UPDATE/DELETE.
func rowsAffectedOrNotFound(res sql.Result, notFound error, msg string, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: "+msg+": rows affected: %w", id, err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}
