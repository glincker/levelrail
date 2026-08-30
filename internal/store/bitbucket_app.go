package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrBitbucketAppConnectionNotFound is returned by
// GetBitbucketAppConnection and DeleteBitbucketAppConnection when no
// bitbucket_app_connections row exists, i.e. the OAuth consumer has
// never been configured.
var ErrBitbucketAppConnectionNotFound = errors.New("store: bitbucket app connection not found")

// BitbucketAppConnection is the single-row bitbucket_app_connections
// table (migrations/0062_bitbucket_app_connection.sql). No secret
// fields: the consumer's own secret and the OAuth access_token/
// refresh_token/token_expires_at live in internal/secrets instead,
// under BitbucketAppSecretsKey(), the same split GitHubAppConnection
// and GitLabAppConnection already establish.
type BitbucketAppConnection struct {
	Key       string
	CreatedAt string
}

// BitbucketAppSecretsKey is the internal/secrets serviceName the OAuth
// consumer's secret and access_token/refresh_token/token_expires_at are
// stored under. A fixed constant, not parameterized: there is only ever
// one Bitbucket connection per control plane, the same reasoning
// GitHubAppSecretsKey/GitLabAppSecretsKey's own doc comments give.
func BitbucketAppSecretsKey() string {
	return "bitbucket-app/connection"
}

// SaveBitbucketAppConnection inserts or replaces the single
// bitbucket_app_connections row (id=1).
func (db *DB) SaveBitbucketAppConnection(ctx context.Context, c BitbucketAppConnection) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO bitbucket_app_connections (id, oauth_key, created_at)
		VALUES (1, ?, ?)
	`, c.Key, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save bitbucket app connection: %w", err)
	}
	return nil
}

// GetBitbucketAppConnection returns the single bitbucket_app_connections
// row, or ErrBitbucketAppConnectionNotFound if none exists.
func (db *DB) GetBitbucketAppConnection(ctx context.Context) (BitbucketAppConnection, error) {
	var c BitbucketAppConnection
	err := db.QueryRowContext(ctx, `
		SELECT oauth_key, created_at FROM bitbucket_app_connections WHERE id = 1
	`).Scan(&c.Key, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BitbucketAppConnection{}, ErrBitbucketAppConnectionNotFound
	}
	if err != nil {
		return BitbucketAppConnection{}, fmt.Errorf("store: get bitbucket app connection: %w", err)
	}
	return c, nil
}

// DeleteBitbucketAppConnection removes the single
// bitbucket_app_connections row. Returns
// ErrBitbucketAppConnectionNotFound if nothing was connected. Like
// DeleteGitLabAppConnection, this does not erase the secrets values
// still held under BitbucketAppSecretsKey(); callers are expected to
// also call BitbucketAppSecrets.DeleteAll.
func (db *DB) DeleteBitbucketAppConnection(ctx context.Context) error {
	res, err := db.ExecContext(ctx, `DELETE FROM bitbucket_app_connections WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("store: delete bitbucket app connection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete bitbucket app connection: %w", err)
	}
	if n == 0 {
		return ErrBitbucketAppConnectionNotFound
	}
	return nil
}
