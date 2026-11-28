package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/glincker/levelrail/internal/cleanup"
	registryreconcile "github.com/glincker/levelrail/internal/reconcile/registry"
	"github.com/glincker/levelrail/internal/registrycatalog"
	"github.com/glincker/levelrail/internal/secrets"
)

// registryPruneClient is the part of *registrycatalog.Client the tag prune needs beyond browsing.
type registryPruneClient interface {
	TagInfo(ctx context.Context, baseURL, username, password, repository, tag string) (registrycatalog.TagInfo, error)
	DeleteManifest(ctx context.Context, baseURL, username, password, repository, digest string) error
}

// errRegistryPruneUnavailable means the built-in registry cannot be pruned right now.
var errRegistryPruneUnavailable = errors.New("the built-in registry is not available for pruning")

// registryTagSource feeds the cleanup center's registry targets from the built-in registry.
type registryTagSource struct{ rt *Router }

// RegistryTagSource returns the cleanup source for the built-in registry's tags and garbage collection.
func (rt *Router) RegistryTagSource() cleanup.RegistryTags { return registryTagSource{rt: rt} }

func (s registryTagSource) login(ctx context.Context, repository string) (user, secret string, err error) {
	rt := s.rt
	if rt.registryCatalog == nil || rt.registryCatalogSecrets == nil || rt.registry == nil {
		return "", "", errRegistryPruneUnavailable
	}
	settings, err := rt.registry.GetRegistrySettings(ctx)
	if err != nil {
		return "", "", fmt.Errorf("get registry settings: %w", err)
	}
	if !settings.Enabled {
		return "", "", errRegistryPruneUnavailable
	}
	// Token auth has no delete scope to mint, so pruning there stays off rather than guessing.
	if rt.registryTokenActive != nil && rt.registryTokenActive() {
		return "", "", errRegistryPruneUnavailable
	}
	user, secret, err = rt.registryCatalogAuth(ctx, settings.Username, repository)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return "", "", errRegistryPruneUnavailable
	}
	return user, secret, err
}

func (s registryTagSource) Repositories(ctx context.Context) ([]string, error) {
	user, secret, err := s.login(ctx, "")
	if errors.Is(err, errRegistryPruneUnavailable) {
		return nil, nil // a disabled registry has nothing to clean, which is not a failed run
	}
	if err != nil {
		return nil, err
	}
	return s.rt.registryCatalog.ListRepositories(ctx, registryCatalogBaseURL(), user, secret)
}

func (s registryTagSource) Tags(ctx context.Context, repository string) ([]registrycatalog.TagInfo, error) {
	pc, ok := s.rt.registryCatalog.(registryPruneClient)
	if !ok {
		return nil, errRegistryPruneUnavailable
	}
	user, secret, err := s.login(ctx, repository)
	if err != nil {
		return nil, err
	}
	names, err := s.rt.registryCatalog.ListTags(ctx, registryCatalogBaseURL(), user, secret, repository)
	if err != nil {
		return nil, err
	}
	out := make([]registrycatalog.TagInfo, 0, len(names))
	for _, n := range names {
		ti, err := pc.TagInfo(ctx, registryCatalogBaseURL(), user, secret, repository, n)
		if err != nil {
			return nil, err
		}
		out = append(out, ti)
	}
	return out, nil
}

func (s registryTagSource) Delete(ctx context.Context, repository, digest string) error {
	pc, ok := s.rt.registryCatalog.(registryPruneClient)
	if !ok {
		return errRegistryPruneUnavailable
	}
	user, secret, err := s.login(ctx, repository)
	if err != nil {
		return err
	}
	return pc.DeleteManifest(ctx, registryCatalogBaseURL(), user, secret, repository, digest)
}

// InUse lists, per repository, every tag some app's image reference points at. A reference by
// digest protects the whole repository, since its tag cannot be told apart.
func (s registryTagSource) InUse(ctx context.Context) (map[string]map[string]bool, error) {
	if s.rt.apps == nil {
		return nil, errRegistryPruneUnavailable
	}
	svcs, err := s.rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	out := map[string]map[string]bool{}
	for _, svc := range svcs {
		repo, tag := cleanup.SplitImageRef(svc.Image)
		if repo == "" {
			continue
		}
		if out[repo] == nil {
			out[repo] = map[string]bool{}
		}
		out[repo][tag] = true
	}
	return out, nil
}

func (s registryTagSource) GarbageCollect(ctx context.Context, dryRun bool) (cleanup.RegistryGCReport, error) {
	rep, err := registryreconcile.RunGC(ctx, dryRun)
	if errors.Is(err, registryreconcile.ErrNotRunning) {
		err = nil
	}
	return cleanup.RegistryGCReport{Blobs: rep.UnreferencedBlobs, Bytes: rep.UnreferencedBytes}, err
}

// registryGCResource is the unreferenced-layer report on the registry status route.
type registryGCResource struct {
	UnreferencedBlobs int    `json:"unreferenced_blobs"`
	UnreferencedBytes int64  `json:"unreferenced_bytes"`
	Error             string `json:"error,omitempty"`
}

// registryGCReport runs a dry-run collection, which writes nothing. It is nil when the registry is off.
func (rt *Router) registryGCReport(ctx context.Context, enabled bool) *registryGCResource {
	if !enabled {
		return nil
	}
	rep, err := registryreconcile.RunGC(ctx, true)
	if err != nil {
		return &registryGCResource{Error: "could not measure unreferenced layers"}
	}
	return &registryGCResource{UnreferencedBlobs: rep.UnreferencedBlobs, UnreferencedBytes: rep.UnreferencedBytes}
}
