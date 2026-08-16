package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GitSource is one app's connected git repository (migrations/0029_service_git_sources.sql):
// what internal/webhook.Config held as static, single-app, server-only
// configuration, now persisted per app so a control plane can auto-deploy
// more than one app from a git push. RepoURL/Branch/BuildType/BuildPath
// are everything internal/api needs to rebuild a spec.Service the same
// way handleTriggerBuild's specServiceFromDesired already does for a
// manual build trigger; the deploy token (private repo auth) and webhook
// HMAC secret are deliberately not fields here, see GitSourceSecretsKey's
// own doc comment for where they actually live.
type GitSource struct {
	ServiceName string
	RepoURL     string
	Branch      string
	BuildType   string
	BuildPath   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ErrGitSourceNotFound is returned by GetGitSource and DeleteGitSource
// when no git source is connected for a given service name.
var ErrGitSourceNotFound = errors.New("store: git source not found")

// GitSourceSecretsKey is the internal/secrets serviceName a git source's
// deploy token ("deploy_token" envKey) and webhook HMAC secret
// ("webhook_secret" envKey) are stored under, the same
// distinct-namespace-from-the-real-service reasoning
// BackupTargetSecretsKey's own doc comment already establishes: a git
// source's own credentials must never share a DEK with the app's actual
// runtime env secrets (service.go's SaveDesiredService/secret_env),
// which live under the plain service name.
func GitSourceSecretsKey(serviceName string) string {
	return "git-source/" + serviceName
}

// SaveGitSource creates a service's git source, or fully replaces it if
// one already exists: unlike SaveBackupTarget/SaveProject (insert-only,
// no legitimate "same id, different contents" case), PUT
// /api/v1/apps/{name}/git-source (internal/api) is a real
// connect-or-edit-the-connection endpoint, so this is an upsert, the
// same shape SaveSecretValue already uses for rotating a secret's value.
// created_at is left untouched by the ON CONFLICT branch, so it reflects
// when the source was first connected even across later edits.
func (db *DB) SaveGitSource(ctx context.Context, g GitSource) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO service_git_sources (service_name, repo_url, branch, build_type, build_path, updated_at)
		VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (service_name) DO UPDATE SET
			repo_url = excluded.repo_url,
			branch = excluded.branch,
			build_type = excluded.build_type,
			build_path = excluded.build_path,
			updated_at = excluded.updated_at
	`, g.ServiceName, g.RepoURL, g.Branch, g.BuildType, g.BuildPath)
	if err != nil {
		return fmt.Errorf("store: save git source for %q: %w", g.ServiceName, err)
	}
	return nil
}

// GetGitSource returns the git source connected to serviceName, or
// ErrGitSourceNotFound if none is.
func (db *DB) GetGitSource(ctx context.Context, serviceName string) (*GitSource, error) {
	row := db.QueryRowContext(ctx, `
		SELECT service_name, repo_url, branch, build_type, build_path, created_at, updated_at
		FROM service_git_sources WHERE service_name = ?
	`, serviceName)
	g, err := scanGitSource(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGitSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get git source for %q: %w", serviceName, err)
	}
	return g, nil
}

// DeleteGitSource disconnects serviceName's git source, returning
// ErrGitSourceNotFound if none exists. Known gap, matching
// DeleteBackupTarget's own honestly-documented one: internal/secrets.Manager
// has no delete/revoke operation today, so the deploy token and webhook
// secret this source's connect flow wrote (GitSourceSecretsKey) remain
// in the secrets store, unreferenced and unreachable through this API
// but not actually erased at rest.
func (db *DB) DeleteGitSource(ctx context.Context, serviceName string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM service_git_sources WHERE service_name = ?`, serviceName)
	if err != nil {
		return fmt.Errorf("store: delete git source for %q: %w", serviceName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete git source for %q: rows affected: %w", serviceName, err)
	}
	if n == 0 {
		return ErrGitSourceNotFound
	}
	return nil
}

func scanGitSource(scan func(dest ...any) error) (*GitSource, error) {
	var (
		g                    GitSource
		createdAt, updatedAt string
	)
	if err := scan(&g.ServiceName, &g.RepoURL, &g.Branch, &g.BuildType, &g.BuildPath, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	g.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	g.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &g, nil
}
