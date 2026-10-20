// Package sharedenv composes internal/store's shared env var tables
// (project_env_vars, organization_env_vars, environment_env_vars) with
// internal/secrets' envelope encryption, so a secret-marked shared env
// var resolves to its decrypted plaintext by the time
// internal/reconcile/application's resolveEnv merges it into a running
// container's environment. *store.DB alone already satisfies
// resolveEnv's ProjectEnvStore/OrganizationEnvStore/EnvironmentEnvLister
// interfaces for the plain-value case (it always has); Resolver is a
// drop-in replacement wherever those are wired up
// (cmd/levelrail/main.go's appControllersFor) that also resolves the
// secret-marked case, without resolveEnv itself needing to know the
// difference.
package sharedenv

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SecretResolver is the narrow surface Resolver needs from
// internal/secrets.Manager: decrypt one secret-marked shared env var's
// value, the same Resolve call internal/reconcile/application's own
// SecretResolver already makes for per-app secrets, database passwords,
// and every other envelope-encrypted credential in this control plane.
type SecretResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Store is the store surface Resolver needs across all three shared-env
// tiers, satisfied structurally by *store.DB.
type Store interface {
	ListProjectEnvVars(ctx context.Context, projectID string) (map[string]string, error)
	ListProjectSecretEnvKeys(ctx context.Context, projectID string) ([]string, error)
	ListOrganizationEnvVarsForProject(ctx context.Context, projectID string) (map[string]string, error)
	GetProjectOrganizationID(ctx context.Context, projectID string) (string, error)
	ListOrganizationSecretEnvKeys(ctx context.Context, orgID string) ([]string, error)
	ListEnvironmentEnvVars(ctx context.Context, environmentID string) (map[string]string, error)
	ListEnvironmentSecretEnvKeys(ctx context.Context, environmentID string) ([]string, error)
}

// Resolver decrypts secret-marked shared env vars fresh on every call
// (every reconcile), never caching a plaintext value across calls, the
// same discipline internal/secrets.Manager.Resolve's own callers already
// hold.
type Resolver struct {
	store   Store
	secrets SecretResolver
}

// NewResolver builds a Resolver. secretsManager may be nil (a control
// plane with no master key configured): secret-marked shared env vars
// are then silently omitted from every List* call below, the same
// "everything except secret resolution still works" shape
// internal/api's own SecretSetter nil case already establishes. Taking
// the concrete *secrets.Manager, not an interface, so a nil check here
// works correctly rather than tripping the typed-nil-interface trap a
// nil-but-typed SecretResolver parameter would.
func NewResolver(store Store, secretsManager *secrets.Manager) *Resolver {
	r := &Resolver{store: store}
	if secretsManager != nil {
		r.secrets = secretsManager
	}
	return r
}

// ListProjectEnvVars satisfies internal/reconcile/application's
// ProjectEnvStore, merging projectID's plain shared env vars with its
// decrypted secret-marked ones.
func (r *Resolver) ListProjectEnvVars(ctx context.Context, projectID string) (map[string]string, error) {
	vars, err := r.store.ListProjectEnvVars(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if r.secrets == nil {
		return vars, nil
	}
	keys, err := r.store.ListProjectSecretEnvKeys(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("sharedenv: list project secret env keys for %q: %w", projectID, err)
	}
	return r.mergeSecrets(ctx, vars, store.ProjectEnvSecretsKey(projectID), keys)
}

// ListOrganizationEnvVarsForProject satisfies internal/reconcile/
// application's OrganizationEnvStore, merging projectID's organization's
// plain shared env vars with its decrypted secret-marked ones.
func (r *Resolver) ListOrganizationEnvVarsForProject(ctx context.Context, projectID string) (map[string]string, error) {
	vars, err := r.store.ListOrganizationEnvVarsForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if r.secrets == nil {
		return vars, nil
	}
	orgID, err := r.store.GetProjectOrganizationID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("sharedenv: get organization for project %q: %w", projectID, err)
	}
	if orgID == "" {
		return vars, nil
	}
	keys, err := r.store.ListOrganizationSecretEnvKeys(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("sharedenv: list organization secret env keys for %q: %w", orgID, err)
	}
	return r.mergeSecrets(ctx, vars, store.OrganizationEnvSecretsKey(orgID), keys)
}

// ListEnvironmentEnvVars satisfies internal/reconcile/application's
// EnvironmentEnvLister, merging environmentID's plain shared env vars
// with its decrypted secret-marked ones.
func (r *Resolver) ListEnvironmentEnvVars(ctx context.Context, environmentID string) (map[string]string, error) {
	vars, err := r.store.ListEnvironmentEnvVars(ctx, environmentID)
	if err != nil {
		return nil, err
	}
	if r.secrets == nil {
		return vars, nil
	}
	keys, err := r.store.ListEnvironmentSecretEnvKeys(ctx, environmentID)
	if err != nil {
		return nil, fmt.Errorf("sharedenv: list environment secret env keys for %q: %w", environmentID, err)
	}
	return r.mergeSecrets(ctx, vars, store.EnvironmentEnvSecretsKey(environmentID), keys)
}

func (r *Resolver) mergeSecrets(ctx context.Context, base map[string]string, namespace string, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return base, nil
	}
	out := make(map[string]string, len(base)+len(keys))
	for k, v := range base {
		out[k] = v
	}
	for _, k := range keys {
		val, err := r.secrets.Resolve(ctx, namespace, k)
		if err != nil {
			return nil, fmt.Errorf("sharedenv: resolve secret env var %q under %q: %w", k, namespace, err)
		}
		out[k] = val
	}
	return out, nil
}
