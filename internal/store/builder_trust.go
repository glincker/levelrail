package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Builder trust tiers a node that builds can hold.
const (
	BuildTrustServer   = "server"
	BuildTrustManaged  = "managed"
	BuildTrustPersonal = "personal"
)

// ValidBuildTrust reports whether v is a known tier.
func ValidBuildTrust(v string) bool {
	return v == BuildTrustServer || v == BuildTrustManaged || v == BuildTrustPersonal
}

// SetNodeBuildTrust sets a node's builder tier, ErrNodeNotFound when the node is unknown.
func (db *DB) SetNodeBuildTrust(ctx context.Context, id, tier string) error {
	if !ValidBuildTrust(tier) {
		return fmt.Errorf("store: unknown build trust %q", tier)
	}
	res, err := db.ExecContext(ctx, `UPDATE nodes SET build_trust = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, tier, id)
	if err != nil {
		return fmt.Errorf("store: set build trust for node %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNodeNotFound
	}
	return nil
}

// ListNodeBuildTrust returns every node's builder tier keyed by node ID.
func (db *DB) ListNodeBuildTrust(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, build_trust FROM nodes`)
	if err != nil {
		return nil, fmt.Errorf("store: list build trust: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, tier string
		if err := rows.Scan(&id, &tier); err != nil {
			return nil, fmt.Errorf("store: scan build trust: %w", err)
		}
		out[id] = tier
	}
	return out, rows.Err()
}

// GetEnvironmentBuilders returns the environment's explicit allow-list (tiers or node IDs), empty when none is set.
func (db *DB) GetEnvironmentBuilders(ctx context.Context, envID string) ([]string, error) {
	var allowed string
	err := db.QueryRowContext(ctx, `SELECT allowed FROM environment_builders WHERE environment_id = ?`, envID).Scan(&allowed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: get environment builders: %w", err)
	}
	if allowed == "" {
		return nil, nil
	}
	return strings.Split(allowed, ","), nil
}

// SetEnvironmentBuilders replaces the allow-list; an empty list restores the default policy.
func (db *DB) SetEnvironmentBuilders(ctx context.Context, envID string, allowed []string, now time.Time) error {
	if len(allowed) == 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM environment_builders WHERE environment_id = ?`, envID); err != nil {
			return fmt.Errorf("store: clear environment builders: %w", err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `INSERT INTO environment_builders (environment_id, allowed, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(environment_id) DO UPDATE SET allowed = excluded.allowed, updated_at = excluded.updated_at`,
		envID, strings.Join(allowed, ","), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set environment builders: %w", err)
	}
	return nil
}
