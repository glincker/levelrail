package pkgregistry

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Source is the store surface the resolver reads.
type Source interface {
	ListPackageRegistries(ctx context.Context) ([]store.PackageRegistry, error)
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
}

// SecretResolver reads a stored credential value.
type SecretResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Resolver turns the registries in scope for one service into build inputs.
type Resolver struct {
	Store   Source
	Secrets SecretResolver
}

func scopeRank(kind string) int {
	switch kind {
	case ScopeApp:
		return 0
	case ScopeProject:
		return 1
	default:
		return 2
	}
}

// BuildSecrets returns the secret mounts and build args for a service's
// Dockerfile build, narrowest scope first so an app can override a
// project or instance default. An empty Output means nothing applies.
func (r *Resolver) BuildSecrets(ctx context.Context, serviceName string) (Output, error) {
	all, err := r.Store.ListPackageRegistries(ctx)
	if err != nil {
		return Output{}, fmt.Errorf("pkgregistry: list registries: %w", err)
	}
	if len(all) == 0 {
		return Output{}, nil
	}
	var projectID, appID string
	svc, err := r.Store.GetDesiredService(ctx, serviceName)
	switch {
	case err == nil:
		projectID, appID = svc.ProjectID, svc.AppID
	case errors.Is(err, store.ErrServiceNotFound):
	default:
		return Output{}, fmt.Errorf("pkgregistry: get service %q: %w", serviceName, err)
	}

	var inScope []store.PackageRegistry
	for _, p := range all {
		switch p.ScopeKind {
		case ScopeApp:
			if p.ScopeRef == serviceName || (appID != "" && p.ScopeRef == appID) {
				inScope = append(inScope, p)
			}
		case ScopeProject:
			if projectID != "" && p.ScopeRef == projectID {
				inScope = append(inScope, p)
			}
		default:
			inScope = append(inScope, p)
		}
	}
	sort.SliceStable(inScope, func(i, j int) bool { return scopeRank(inScope[i].ScopeKind) < scopeRank(inScope[j].ScopeKind) })

	entries := make([]Entry, 0, len(inScope))
	for _, p := range inScope {
		secret, err := r.Secrets.Resolve(ctx, store.PackageRegistrySecretsKey(p.ID), store.PackageRegistrySecretEnvKey)
		if err != nil {
			return Output{}, fmt.Errorf("pkgregistry: resolve credential for registry %q: %w", p.Name, err)
		}
		entries = append(entries, Entry{
			Ecosystem: p.Ecosystem, URL: p.URL, Username: p.Username,
			Secret: secret, ModulePatterns: p.ModulePatterns,
		})
	}
	return Render(entries)
}
