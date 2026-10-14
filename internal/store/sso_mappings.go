package store

import (
	"context"
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
