package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	// ErrSCIMGroupNotFound is returned when no directory group has the ID.
	ErrSCIMGroupNotFound = errors.New("store: scim group not found")
	// ErrSCIMGroupNameTaken is returned when the display name is in use.
	ErrSCIMGroupNameTaken = errors.New("store: a scim group with that name already exists")
	// ErrSCIMMemberUnknown is returned when a member ID names no user.
	ErrSCIMMemberUnknown = errors.New("store: scim group member is not a user")
)

// SCIM group filter attributes.
const (
	SCIMGroupFilterDisplayName = "displayName"
	SCIMGroupFilterExternalID  = "externalId"
	SCIMGroupFilterID          = "id"
)

// SCIMGroup is a directory group and its member user IDs.
type SCIMGroup struct {
	ID          string
	DisplayName string
	ExternalID  string
	Members     []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MemberChange is one ordered change to a group's members.
type MemberChange struct {
	Replace bool
	Add     []string
	Remove  []string
}

func scanSCIMGroup(scan func(dest ...any) error) (SCIMGroup, error) {
	var (
		g                SCIMGroup
		created, updated string
	)
	err := scan(&g.ID, &g.DisplayName, &g.ExternalID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return SCIMGroup{}, ErrSCIMGroupNotFound
	}
	if err != nil {
		return SCIMGroup{}, fmt.Errorf("store: scan scim group: %w", err)
	}
	if g.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return SCIMGroup{}, fmt.Errorf("store: scim group created_at: %w", err)
	}
	if g.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return SCIMGroup{}, fmt.Errorf("store: scim group updated_at: %w", err)
	}
	return g, nil
}

func (db *DB) scimGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT user_id FROM scim_group_members WHERE group_id = ? ORDER BY user_id`, groupID)
	if err != nil {
		return nil, fmt.Errorf("store: scim group members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scim group members: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetSCIMGroup returns a group with its members.
func (db *DB) GetSCIMGroup(ctx context.Context, id string) (SCIMGroup, error) {
	g, err := scanSCIMGroup(db.QueryRowContext(ctx, `SELECT id, display_name, external_id, created_at, updated_at FROM scim_groups WHERE id = ?`, id).Scan)
	if err != nil {
		return SCIMGroup{}, err
	}
	g.Members, err = db.scimGroupMembers(ctx, id)
	return g, err
}

// ListSCIMGroups returns one page of groups with members and the total matching.
func (db *DB) ListSCIMGroups(ctx context.Context, attr, value string, offset, limit int) ([]SCIMGroup, int, error) {
	where, args := "", []any{}
	switch attr {
	case "":
	case SCIMGroupFilterDisplayName:
		where, args = ` WHERE display_name = ?`, []any{value}
	case SCIMGroupFilterExternalID:
		where, args = ` WHERE external_id = ?`, []any{value}
	case SCIMGroupFilterID:
		where, args = ` WHERE id = ?`, []any{value}
	default:
		return nil, 0, fmt.Errorf("store: unsupported scim group filter %q", attr)
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM scim_groups`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count scim groups: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT id, display_name, external_id, created_at, updated_at FROM scim_groups`+where+
		` ORDER BY created_at, id LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list scim groups: %w", err)
	}
	var out []SCIMGroup
	for rows.Next() {
		g, err := scanSCIMGroup(rows.Scan)
		if err != nil {
			_ = rows.Close()
			return nil, 0, err
		}
		out = append(out, g)
	}
	_ = rows.Close()
	for i := range out {
		if out[i].Members, err = db.scimGroupMembers(ctx, out[i].ID); err != nil {
			return nil, 0, err
		}
	}
	return out, total, nil
}

// SaveSCIMGroup creates or renames a group and applies member changes in
// order, all in one transaction. It returns every user whose membership moved.
func (db *DB) SaveSCIMGroup(ctx context.Context, g SCIMGroup, create bool, changes []MemberChange, now time.Time) ([]string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: save scim group: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var res sql.Result
	if create {
		res, err = tx.ExecContext(ctx, `INSERT INTO scim_groups (id, display_name, external_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			g.ID, g.DisplayName, g.ExternalID, formatTime(now), formatTime(now))
	} else {
		res, err = tx.ExecContext(ctx, `UPDATE scim_groups SET display_name = ?, external_id = ?, updated_at = ? WHERE id = ?`,
			g.DisplayName, g.ExternalID, formatTime(now), g.ID)
	}
	if isUniqueViolation(err) {
		return nil, ErrSCIMGroupNameTaken
	}
	if err != nil {
		return nil, fmt.Errorf("store: save scim group %q: %w", g.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrSCIMGroupNotFound
	}
	touched, err := applyMemberChanges(ctx, tx, g.ID, changes)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: save scim group %q: %w", g.ID, err)
	}
	return touched, nil
}

func applyMemberChanges(ctx context.Context, tx *sql.Tx, groupID string, changes []MemberChange) ([]string, error) {
	touched := map[string]bool{}
	for _, c := range changes {
		if c.Replace {
			if err := collectMembers(ctx, tx, groupID, touched); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM scim_group_members WHERE group_id = ?`, groupID); err != nil {
				return nil, fmt.Errorf("store: replace scim members: %w", err)
			}
		}
		for _, id := range c.Add {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = ?`, id).Scan(&n); err != nil {
				return nil, fmt.Errorf("store: check scim member: %w", err)
			}
			if n == 0 {
				return nil, fmt.Errorf("%w: %s", ErrSCIMMemberUnknown, id)
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO scim_group_members (group_id, user_id) VALUES (?, ?)`, groupID, id); err != nil {
				return nil, fmt.Errorf("store: add scim member: %w", err)
			}
			touched[id] = true
		}
		for _, id := range c.Remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM scim_group_members WHERE group_id = ? AND user_id = ?`, groupID, id); err != nil {
				return nil, fmt.Errorf("store: remove scim member: %w", err)
			}
			touched[id] = true
		}
	}
	out := make([]string, 0, len(touched))
	for id := range touched {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func collectMembers(ctx context.Context, tx *sql.Tx, groupID string, into map[string]bool) error {
	rows, err := tx.QueryContext(ctx, `SELECT user_id FROM scim_group_members WHERE group_id = ?`, groupID)
	if err != nil {
		return fmt.Errorf("store: scim members: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("store: scim members: %w", err)
		}
		into[id] = true
	}
	return rows.Err()
}

// DeleteSCIMGroup removes a group and returns its former members.
func (db *DB) DeleteSCIMGroup(ctx context.Context, id string) ([]string, error) {
	members, err := db.scimGroupMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	res, err := db.ExecContext(ctx, `DELETE FROM scim_groups WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: delete scim group %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrSCIMGroupNotFound
	}
	return members, nil
}

// SCIMGroupNamesForUser lists the display names of the user's directory groups.
func (db *DB) SCIMGroupNamesForUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT g.display_name FROM scim_groups g
		JOIN scim_group_members m ON m.group_id = g.id WHERE m.user_id = ? ORDER BY g.display_name`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: scim groups for user: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scim groups for user: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// SCIMGroupRefsForUser returns id and name pairs of the user's directory groups.
func (db *DB) SCIMGroupRefsForUser(ctx context.Context, userID string) ([][2]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT g.id, g.display_name FROM scim_groups g
		JOIN scim_group_members m ON m.group_id = g.id WHERE m.user_id = ? ORDER BY g.display_name`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: scim group refs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out [][2]string
	for rows.Next() {
		var ref [2]string
		if err := rows.Scan(&ref[0], &ref[1]); err != nil {
			return nil, fmt.Errorf("store: scim group refs: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
