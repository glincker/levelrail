package store

import (
	"context"
	"fmt"
	"time"
)

// ForgeDeployment records a deployment created on a git forge (GitHub
// Deployments) for an app deploy, so a later deploy can mark it inactive.
type ForgeDeployment struct {
	ID          string
	AppName     string
	Environment string
	Provider    string
	ExternalID  int64
	CommitSHA   string
	State       string
	Warning     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CreateForgeDeployment inserts d.
func (db *DB) CreateForgeDeployment(ctx context.Context, d ForgeDeployment) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO forge_deployments (id, app_name, environment, provider, external_id, commit_sha, state, warning, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, d.ID, d.AppName, d.Environment, d.Provider, d.ExternalID, d.CommitSHA, d.State, d.Warning, formatTime(d.CreatedAt), formatTime(d.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create forge deployment: %w", err)
	}
	return nil
}

// SetForgeDeploymentState updates a deployment's state and warning.
func (db *DB) SetForgeDeploymentState(ctx context.Context, id, state, warning string, at time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE forge_deployments SET state = ?, warning = ?, updated_at = ? WHERE id = ?`, state, warning, formatTime(at), id)
	if err != nil {
		return fmt.Errorf("store: set forge deployment %q state: %w", id, err)
	}
	return nil
}

// ListForgeDeploymentsByState returns an app environment's deployments in
// state, excluding excludeID, newest first.
func (db *DB) ListForgeDeploymentsByState(ctx context.Context, app, environment, state, excludeID string) ([]ForgeDeployment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, app_name, environment, provider, external_id, commit_sha, state, warning, created_at, updated_at
		FROM forge_deployments WHERE app_name = ? AND environment = ? AND state = ? AND id != ?
		ORDER BY created_at DESC LIMIT 50
	`, app, environment, state, excludeID)
	if err != nil {
		return nil, fmt.Errorf("store: list forge deployments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ForgeDeployment
	for rows.Next() {
		var d ForgeDeployment
		var created, updated string
		if err := rows.Scan(&d.ID, &d.AppName, &d.Environment, &d.Provider, &d.ExternalID, &d.CommitSHA, &d.State, &d.Warning, &created, &updated); err != nil {
			return nil, fmt.Errorf("store: scan forge deployment: %w", err)
		}
		if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse forge deployment created_at: %w", err)
		}
		if d.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
			return nil, fmt.Errorf("store: parse forge deployment updated_at: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
