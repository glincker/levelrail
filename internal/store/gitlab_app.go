package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrGitLabAppConnectionNotFound is returned by GetGitLabAppConnection
// and DeleteGitLabAppConnection when no gitlab_app_connections row
// exists, i.e. the OAuth Application has never been registered.
var ErrGitLabAppConnectionNotFound = errors.New("store: gitlab app connection not found")

// GitLabAppConnection is the single-row gitlab_app_connections table
// (migrations/0044_gitlab_app_connection.sql). No credential fields:
// client_secret and the OAuth access_token/refresh_token live in
// internal/secrets instead, under GitLabAppSecretsKey().
type GitLabAppConnection struct {
	InstanceURL string
	ClientID    string
	CreatedAt   string
}

// GitLabAppSecretsKey is the internal/secrets serviceName the OAuth
// Application's client_secret and access_token/refresh_token/
// token_expires_at are stored under. A fixed constant, not
// parameterized: there is only ever one GitLab connection per control
// plane, the same reasoning GitHubAppSecretsKey's own doc comment gives.
func GitLabAppSecretsKey() string {
	return "gitlab-app/connection"
}

// SaveGitLabAppConnection inserts or replaces the single
// gitlab_app_connections row (id=1).
func (db *DB) SaveGitLabAppConnection(ctx context.Context, c GitLabAppConnection) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO gitlab_app_connections (id, instance_url, client_id, created_at)
		VALUES (1, ?, ?, ?)
	`, c.InstanceURL, c.ClientID, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save gitlab app connection: %w", err)
	}
	return nil
}

// GetGitLabAppConnection returns the single gitlab_app_connections row,
// or ErrGitLabAppConnectionNotFound if none exists.
func (db *DB) GetGitLabAppConnection(ctx context.Context) (GitLabAppConnection, error) {
	var c GitLabAppConnection
	err := db.QueryRowContext(ctx, `
		SELECT instance_url, client_id, created_at FROM gitlab_app_connections WHERE id = 1
	`).Scan(&c.InstanceURL, &c.ClientID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GitLabAppConnection{}, ErrGitLabAppConnectionNotFound
	}
	if err != nil {
		return GitLabAppConnection{}, fmt.Errorf("store: get gitlab app connection: %w", err)
	}
	return c, nil
}

// DeleteGitLabAppConnection removes the single gitlab_app_connections
// row. Returns ErrGitLabAppConnectionNotFound if nothing was connected.
// Like DeleteGitHubAppConnection, this does not erase the secrets
// values (client_secret/access_token/refresh_token) still held under
// GitLabAppSecretsKey(); callers are expected to also call
// GitLabAppSecrets.DeleteAll.
func (db *DB) DeleteGitLabAppConnection(ctx context.Context) error {
	res, err := db.ExecContext(ctx, `DELETE FROM gitlab_app_connections WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("store: delete gitlab app connection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete gitlab app connection: %w", err)
	}
	if n == 0 {
		return ErrGitLabAppConnectionNotFound
	}
	return nil
}
