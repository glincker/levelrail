package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
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
	// AdditionalServices lets one push fan out to sibling services under
	// the same store.App (apps_group.go): keyed by the sibling
	// DesiredService's own name, each entry carries just enough to call
	// specServiceFromDesired for it (migrations/0057_git_source_additional_services.sql).
	// A monorepo's other services rarely share this app's exact build
	// config, so this is a map, not a single shared BuildType/BuildPath.
	//
	// Mutually exclusive with Services (internal/api's
	// validateAdditionalServices enforces this at write time): an app
	// that has grown a real Services map fans out through that instead,
	// see Services's own doc comment.
	AdditionalServices map[string]GitSourceBuild
	// Services is an app.yaml-style services: map, persisted so a push
	// can re-run the same fan-out deploy.Pipeline.DeploySpec (internal/
	// api/apps_multi.go's handleDeploySpec) already performs for a
	// manual/API-triggered multi-service deploy, without an operator
	// re-submitting it by hand on every commit
	// (migrations/0063_git_source_services_spec.sql). Empty/nil for
	// every git source created before this field existed, and for any
	// git source that only ever used AdditionalServices: a webhook keeps
	// walking AdditionalServices exactly as before when this is empty,
	// see handleGitPushWebhook's own doc comment.
	Services  map[string]spec.Service
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GitSourceBuild is one additional service's own build config within
// GitSource.AdditionalServices, mirroring this same struct's
// BuildType/BuildPath fields but scoped to a single sibling.
type GitSourceBuild struct {
	BuildType string `json:"build_type"`
	BuildPath string `json:"build_path,omitempty"`
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
	additionalJSON, err := marshalGitSourceAdditionalServices(g.AdditionalServices)
	if err != nil {
		return fmt.Errorf("store: save git source for %q: %w", g.ServiceName, err)
	}
	servicesJSON, err := marshalGitSourceServices(g.Services)
	if err != nil {
		return fmt.Errorf("store: save git source for %q: %w", g.ServiceName, err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO service_git_sources (service_name, repo_url, branch, build_type, build_path, additional_services, services_spec, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (service_name) DO UPDATE SET
			repo_url = excluded.repo_url,
			branch = excluded.branch,
			build_type = excluded.build_type,
			build_path = excluded.build_path,
			additional_services = excluded.additional_services,
			services_spec = excluded.services_spec,
			updated_at = excluded.updated_at
	`, g.ServiceName, g.RepoURL, g.Branch, g.BuildType, g.BuildPath, additionalJSON, servicesJSON)
	if err != nil {
		return fmt.Errorf("store: save git source for %q: %w", g.ServiceName, err)
	}
	return nil
}

// GetGitSource returns the git source connected to serviceName, or
// ErrGitSourceNotFound if none is.
func (db *DB) GetGitSource(ctx context.Context, serviceName string) (*GitSource, error) {
	row := db.QueryRowContext(ctx, `
		SELECT service_name, repo_url, branch, build_type, build_path, additional_services, services_spec, created_at, updated_at
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

// marshalGitSourceAdditionalServices serializes m for storage, defaulting
// a nil/empty map to "{}" so the column never holds SQL NULL or an empty
// string, either of which would fail scanGitSource's own json.Unmarshal.
func marshalGitSourceAdditionalServices(m map[string]GitSourceBuild) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal additional services: %w", err)
	}
	return string(b), nil
}

// marshalGitSourceServices serializes m for storage, defaulting a
// nil/empty map to "{}" the same way marshalGitSourceAdditionalServices
// does, for the identical reason (scanGitSource's json.Unmarshal must
// never see SQL NULL or an empty string).
func marshalGitSourceServices(m map[string]spec.Service) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal services: %w", err)
	}
	return string(b), nil
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
		g                       GitSource
		additionalJSON, svcJSON string
		createdAt, updatedAt    string
	)
	if err := scan(&g.ServiceName, &g.RepoURL, &g.Branch, &g.BuildType, &g.BuildPath, &additionalJSON, &svcJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(additionalJSON), &g.AdditionalServices); err != nil {
		return nil, fmt.Errorf("unmarshal additional_services: %w", err)
	}
	if err := json.Unmarshal([]byte(svcJSON), &g.Services); err != nil {
		return nil, fmt.Errorf("unmarshal services_spec: %w", err)
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
