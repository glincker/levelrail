package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrGitHubAppConnectionNotFound is returned by GetGitHubAppConnection
// and UpdateGitHubAppInstallation/DeleteGitHubAppConnection when no
// github_app_connections row exists, i.e. the App has never been
// registered through the manifest flow.
var ErrGitHubAppConnectionNotFound = errors.New("store: github app connection not found")

// GitHubAppConnection is the single-row github_app_connections table
// (migrations/0034_github_app_connection.sql). No secret fields: see
// that migration's own comment for why client_secret/webhook_secret/the
// PEM private key live in internal/secrets instead, under
// GitHubAppSecretsKey().
type GitHubAppConnection struct {
	AppID    int64
	ClientID string
	// InstanceURL is the GitHub instance this App is registered
	// against: "https://github.com" for every connection until GitHub
	// Enterprise Server support (migrations/0061), a real GHES base URL
	// after. Never empty: the migration backfills the column, and
	// SaveGitHubAppConnection requires a caller to pass one.
	InstanceURL string
	CreatedAt   string
	// InstallationID and AccountLogin are nil until the App has been
	// installed on a GitHub account/org (the redirect
	// GET /api/v1/github-app/installed records). Pointers, not a
	// zero-value sentinel, since 0 is not a distinguishable "unset" value
	// for an installation ID.
	InstallationID *int64
	AccountLogin   *string
}

// GitHubAppSecretsKey is the internal/secrets serviceName the GitHub
// App's client_secret, webhook_secret, and PEM private key are stored
// under (envKeys "client_secret", "webhook_secret", "private_key").
// Mirrors BackupTargetSecretsKey's exact shape: a function, not a
// literal repeated at each call site, so internal/api and
// internal/githubapp can never drift into computing this two different
// ways.
//
// A fixed constant, not parameterized by an ID like
// BackupTargetSecretsKey(targetID) is: there is only ever one GitHub
// App connection per control plane (github_app_connections is a
// singleton table, id=1 only), unlike backup targets which are
// one-per-connected-bucket. The "/" makes this string structurally
// impossible to collide with a real service name: every real
// desired_services row name is validated as a Docker-container/
// DNS-label-safe token (see internal/spec's service name validation)
// and can never contain a "/", the same reasoning
// BackupTargetSecretsKey's own doc comment gives for its
// "backup-target/"+targetID shape.
func GitHubAppSecretsKey() string {
	return "github-app/connection"
}

// SaveGitHubAppConnection inserts or replaces the single
// github_app_connections row (id=1). Called once, right after a
// successful manifest code exchange
// (internal/api's handleGitHubAppCallback): InstallationID and
// AccountLogin are always nil at this point, since installation happens
// in a separate, later redirect. INSERT OR REPLACE rather than a plain
// INSERT: a second registration (after a prior connection was removed
// via DeleteGitHubAppConnection) must succeed without a caller having to
// know whether a row already exists.
func (db *DB) SaveGitHubAppConnection(ctx context.Context, c GitHubAppConnection) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO github_app_connections
			(id, app_id, client_id, instance_url, installation_id, account_login, created_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
	`, c.AppID, c.ClientID, c.InstanceURL, nullableInt64(c.InstallationID), nullableString(c.AccountLogin), c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save github app connection: %w", err)
	}
	return nil
}

// GetGitHubAppConnection returns the single github_app_connections row,
// or ErrGitHubAppConnectionNotFound if the App has never been
// registered.
func (db *DB) GetGitHubAppConnection(ctx context.Context) (GitHubAppConnection, error) {
	var (
		c              GitHubAppConnection
		installationID sql.NullInt64
		accountLogin   sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT app_id, client_id, instance_url, installation_id, account_login, created_at
		FROM github_app_connections
		WHERE id = 1
	`).Scan(&c.AppID, &c.ClientID, &c.InstanceURL, &installationID, &accountLogin, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GitHubAppConnection{}, ErrGitHubAppConnectionNotFound
	}
	if err != nil {
		return GitHubAppConnection{}, fmt.Errorf("store: get github app connection: %w", err)
	}
	if installationID.Valid {
		v := installationID.Int64
		c.InstallationID = &v
	}
	if accountLogin.Valid {
		v := accountLogin.String
		c.AccountLogin = &v
	}
	return c, nil
}

// UpdateGitHubAppInstallation records the installation_id and account
// login GitHub's post-install redirect
// (GET /api/v1/github-app/installed) reports. Returns
// ErrGitHubAppConnectionNotFound if the App itself was never
// registered: an installation can't exist without an App to install.
func (db *DB) UpdateGitHubAppInstallation(ctx context.Context, installationID int64, accountLogin string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE github_app_connections
		SET installation_id = ?, account_login = ?
		WHERE id = 1
	`, installationID, accountLogin)
	if err != nil {
		return fmt.Errorf("store: update github app installation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update github app installation: %w", err)
	}
	if n == 0 {
		return ErrGitHubAppConnectionNotFound
	}
	return nil
}

// DeleteGitHubAppConnection removes the single github_app_connections
// row, the "disconnect" action. Returns ErrGitHubAppConnectionNotFound
// if nothing was connected.
//
// This does not reach out to GitHub to delete or uninstall the App
// there, and it does not delete the client_secret/webhook_secret/
// private_key values internal/secrets still holds under
// GitHubAppSecretsKey(): the same known, documented gap
// DeleteBackupTarget's own doc comment already accepts for backup
// target credentials, since internal/secrets.Manager has no delete
// operation today. A re-registration (SaveGitHubAppConnection) simply
// overwrites those secret values with the new App's own.
func (db *DB) DeleteGitHubAppConnection(ctx context.Context) error {
	res, err := db.ExecContext(ctx, `DELETE FROM github_app_connections WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("store: delete github app connection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete github app connection: %w", err)
	}
	if n == 0 {
		return ErrGitHubAppConnectionNotFound
	}
	return nil
}

// nullableInt64 converts a possibly-nil *int64 field to the
// database/sql type ExecContext needs to write either a real value or a
// genuine SQL NULL, mirroring nullableString below for the same reason.
func nullableInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

// nullableString converts a possibly-nil *string field the same way
// nullableInt64 does.
func nullableString(v *string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *v, Valid: true}
}
