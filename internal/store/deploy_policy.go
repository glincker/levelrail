package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/policy"
)

// DeployPolicy is one scope's stored rule set.
type DeployPolicy struct {
	Scope     string
	ScopeID   string
	Rules     policy.Set
	UpdatedBy string
	UpdatedAt time.Time
}

// GetDeployPolicy returns a scope's rules, or an empty set when none exist.
func (db *DB) GetDeployPolicy(ctx context.Context, scope, scopeID string) (policy.Set, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT rules FROM deploy_policies WHERE scope = ? AND scope_id = ?`, scope, scopeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return policy.Set{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get deploy policy %s/%s: %w", scope, scopeID, err)
	}
	var set policy.Set
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return nil, fmt.Errorf("store: decode deploy policy %s/%s: %w", scope, scopeID, err)
	}
	return set, nil
}

// SetDeployPolicy replaces a scope's rules.
func (db *DB) SetDeployPolicy(ctx context.Context, scope, scopeID string, set policy.Set, by string) error {
	raw, err := json.Marshal(set)
	if err != nil {
		return fmt.Errorf("store: encode deploy policy: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO deploy_policies (scope, scope_id, rules, updated_by, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(scope, scope_id) DO UPDATE SET rules = excluded.rules, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		scope, scopeID, string(raw), by, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set deploy policy %s/%s: %w", scope, scopeID, err)
	}
	return nil
}

// DeleteDeployPolicy removes a scope's rules.
func (db *DB) DeleteDeployPolicy(ctx context.Context, scope, scopeID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_policies WHERE scope = ? AND scope_id = ?`, scope, scopeID); err != nil {
		return fmt.Errorf("store: delete deploy policy %s/%s: %w", scope, scopeID, err)
	}
	return nil
}

// ListDeployPolicies returns every stored scope, oldest scope kind first.
func (db *DB) ListDeployPolicies(ctx context.Context) ([]DeployPolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT scope, scope_id, rules, updated_by, updated_at FROM deploy_policies ORDER BY scope, scope_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployPolicy
	for rows.Next() {
		var (
			p            DeployPolicy
			raw, updated string
		)
		if err := rows.Scan(&p.Scope, &p.ScopeID, &raw, &p.UpdatedBy, &updated); err != nil {
			return nil, fmt.Errorf("store: scan deploy policy: %w", err)
		}
		if err := json.Unmarshal([]byte(raw), &p.Rules); err != nil {
			return nil, fmt.Errorf("store: decode deploy policy %s/%s: %w", p.Scope, p.ScopeID, err)
		}
		if t, err := time.Parse(time.RFC3339Nano, updated); err == nil {
			p.UpdatedAt = t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
