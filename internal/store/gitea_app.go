package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrGiteaAppConnectionNotFound is returned by GetGiteaAppConnection and
// DeleteGiteaAppConnection when no gitea_app_connections row exists,
// i.e. the OAuth2 application has never been registered.
var ErrGiteaAppConnectionNotFound = errors.New("store: gitea app connection not found")

// GiteaAppConnection is the single-row gitea_app_connections table
// (migrations/0114_gitea_app_connection.sql). No credential fields:
// client_secret and the OAuth access_token/refresh_token live in
// internal/secrets instead, under GiteaAppSecretsKey(), the same split
// GitLabAppConnection's own doc comment establishes.
type GiteaAppConnection struct {
	InstanceURL string
	ClientID    string
	CreatedAt   string
}

// GiteaAppSecretsKey is the internal/secrets serviceName the OAuth2
// application's client_secret and access_token/refresh_token/
// token_expires_at are stored under. A fixed constant, not
// parameterized: there is only ever one Gitea connection per control
// plane, the same reasoning GitLabAppSecretsKey's own doc comment gives.
func GiteaAppSecretsKey() string {
	return "gitea-app/connection"
}

// SaveGiteaAppConnection inserts or replaces the single
// gitea_app_connections row (id=1).
func (db *DB) SaveGiteaAppConnection(ctx context.Context, c GiteaAppConnection) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO gitea_app_connections (id, instance_url, client_id, created_at)
		VALUES (1, ?, ?, ?)
	`, c.InstanceURL, c.ClientID, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save gitea app connection: %w", err)
	}
	return nil
}

// GetGiteaAppConnection returns the single gitea_app_connections row, or
// ErrGiteaAppConnectionNotFound if none exists.
func (db *DB) GetGiteaAppConnection(ctx context.Context) (GiteaAppConnection, error) {
	var c GiteaAppConnection
	err := db.QueryRowContext(ctx, `
		SELECT instance_url, client_id, created_at FROM gitea_app_connections WHERE id = 1
	`).Scan(&c.InstanceURL, &c.ClientID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GiteaAppConnection{}, ErrGiteaAppConnectionNotFound
	}
	if err != nil {
		return GiteaAppConnection{}, fmt.Errorf("store: get gitea app connection: %w", err)
	}
	return c, nil
}

// DeleteGiteaAppConnection removes the single gitea_app_connections row.
// Returns ErrGiteaAppConnectionNotFound if nothing was connected. Like
// DeleteGitLabAppConnection, this does not erase the secrets values
// (client_secret/access_token/refresh_token) still held under
// GiteaAppSecretsKey(); callers are expected to also call
// GiteaAppSecrets.DeleteAll.
func (db *DB) DeleteGiteaAppConnection(ctx context.Context) error {
	res, err := db.ExecContext(ctx, `DELETE FROM gitea_app_connections WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("store: delete gitea app connection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete gitea app connection: %w", err)
	}
	if n == 0 {
		return ErrGiteaAppConnectionNotFound
	}
	return nil
}
