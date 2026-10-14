package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SCIM user filter attributes the directory can be searched by.
const (
	SCIMFilterUserName   = "userName"
	SCIMFilterExternalID = "externalId"
	SCIMFilterEmail      = "emails.value"
	SCIMFilterID         = "id"
)

// SCIMUser is a platform user as the directory sees it. Users the
// directory never created still appear, so an IdP can match and adopt them.
type SCIMUser struct {
	UserID      string
	Email       string
	DisplayName string
	ExternalID  string
	UserName    string
	GivenName   string
	FamilyName  string
	Disabled    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const scimUserSelect = `SELECT u.id, u.email, u.display_name, COALESCE(s.external_id, ''), COALESCE(s.user_name, u.email),
	COALESCE(s.given_name, ''), COALESCE(s.family_name, ''), d.user_id IS NOT NULL, u.created_at, COALESCE(s.updated_at, u.created_at)
	FROM users u
	LEFT JOIN scim_users s ON s.user_id = u.id
	LEFT JOIN disabled_users d ON d.user_id = u.id
	WHERE COALESCE(s.deleted, 0) = 0`

func scanSCIMUser(scan func(dest ...any) error) (SCIMUser, error) {
	var (
		u                SCIMUser
		created, updated string
	)
	err := scan(&u.UserID, &u.Email, &u.DisplayName, &u.ExternalID, &u.UserName, &u.GivenName, &u.FamilyName, &u.Disabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return SCIMUser{}, ErrUserNotFound
	}
	if err != nil {
		return SCIMUser{}, fmt.Errorf("store: scan scim user: %w", err)
	}
	if u.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return SCIMUser{}, fmt.Errorf("store: scim user created_at: %w", err)
	}
	if u.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return SCIMUser{}, fmt.Errorf("store: scim user updated_at: %w", err)
	}
	return u, nil
}

func scimUserFilter(attr string) (string, error) {
	switch attr {
	case "":
		return "", nil
	case SCIMFilterUserName:
		return ` AND lower(COALESCE(s.user_name, u.email)) = lower(?)`, nil
	case SCIMFilterExternalID:
		return ` AND s.external_id = ?`, nil
	case SCIMFilterEmail:
		return ` AND lower(u.email) = lower(?)`, nil
	case SCIMFilterID:
		return ` AND u.id = ?`, nil
	}
	return "", fmt.Errorf("store: unsupported scim user filter %q", attr)
}

// ListSCIMUsers returns one page of directory users, oldest first, and the
// total matching. offset is zero-based.
func (db *DB) ListSCIMUsers(ctx context.Context, attr, value string, offset, limit int) ([]SCIMUser, int, error) {
	where, err := scimUserFilter(attr)
	if err != nil {
		return nil, 0, err
	}
	args := []any{}
	if attr != "" {
		args = append(args, value)
	}
	var total int
	countQ := `SELECT COUNT(*) FROM users u LEFT JOIN scim_users s ON s.user_id = u.id WHERE COALESCE(s.deleted, 0) = 0` + where
	if err := db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count scim users: %w", err)
	}
	rows, err := db.QueryContext(ctx, scimUserSelect+where+` ORDER BY u.created_at, u.id LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list scim users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SCIMUser
	for rows.Next() {
		u, err := scanSCIMUser(rows.Scan)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("store: list scim users: %w", err)
	}
	return out, total, nil
}

// GetSCIMUser returns one directory user; a directory-deleted user is not found.
func (db *DB) GetSCIMUser(ctx context.Context, userID string) (SCIMUser, error) {
	return scanSCIMUser(db.QueryRowContext(ctx, scimUserSelect+` AND u.id = ?`, userID).Scan)
}

// SaveSCIMUser writes the directory attributes of an existing user and its
// display name, and clears a directory delete.
func (db *DB) SaveSCIMUser(ctx context.Context, u SCIMUser, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save scim user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE users SET display_name = ? WHERE id = ?`, u.DisplayName, u.UserID)
	if err != nil {
		return fmt.Errorf("store: save scim user %q: %w", u.UserID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUserNotFound
	}
	if err := upsertSCIMUser(ctx, tx, u, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save scim user %q: %w", u.UserID, err)
	}
	return nil
}

func upsertSCIMUser(ctx context.Context, tx *sql.Tx, u SCIMUser, now time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO scim_users (user_id, external_id, user_name, given_name, family_name, deleted, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET external_id = excluded.external_id, user_name = excluded.user_name,
			given_name = excluded.given_name, family_name = excluded.family_name, deleted = 0, updated_at = excluded.updated_at`,
		u.UserID, u.ExternalID, u.UserName, u.GivenName, u.FamilyName, formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("store: upsert scim user %q: %w", u.UserID, err)
	}
	return nil
}

// MarkSCIMUserDeleted hides the user from the directory, drops its directory
// group memberships and disables the account, in one transaction.
func (db *DB) MarkSCIMUserDeleted(ctx context.Context, userID string, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete scim user: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO disabled_users (user_id, reason, disabled_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET reason = excluded.reason`, userID, DisabledReasonSCIMDeleted, formatTime(now)); err != nil {
		return fmt.Errorf("store: delete scim user %q: %w", userID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO scim_users (user_id, user_name, deleted, created_at, updated_at)
		VALUES (?, (SELECT email FROM users WHERE id = ?), 1, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET deleted = 1, updated_at = excluded.updated_at`, userID, userID, formatTime(now), formatTime(now)); err != nil {
		return fmt.Errorf("store: delete scim user %q: %w", userID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM scim_group_members WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: delete scim user %q: %w", userID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete scim user %q: %w", userID, err)
	}
	return nil
}
