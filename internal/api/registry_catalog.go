package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	registryreconcile "github.com/GLINCKER/levelrail/internal/reconcile/registry"
	"github.com/GLINCKER/levelrail/internal/registrycatalog"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RegistryCatalogClient is the surface GET /api/v1/registry/repositories
// and GET /api/v1/registry/tags need: query a Docker Registry HTTP API
// v2 server's catalog (_catalog, <name>/tags/list) over Basic Auth.
// Scoped to Levelrail's own built-in registry only (registryCatalogBaseURL
// below is fixed to its loopback address, never an operator-supplied
// host): querying an arbitrary external registry's catalog is a larger,
// separately-scoped feature, since many registries block or rate-limit
// catalog listing and a stored RegistryCredential row carries no
// guarantee its scope even permits it. *registrycatalog.Client satisfies
// this structurally.
type RegistryCatalogClient interface {
	ListRepositories(ctx context.Context, baseURL, username, password string) ([]string, error)
	ListTags(ctx context.Context, baseURL, username, password, repository string) ([]string, error)
}

// RegistryCatalogSecrets is the surface the catalog handlers need from
// internal/secrets.Manager: resolve the built-in registry's generated
// password to authenticate the catalog query server-side. Never echoed
// back to the caller, the same boundary
// RegistryCredentialSecretsSetter.Resolve already draws for
// handleTestRegistryCredential.
type RegistryCatalogSecrets interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// registryCatalogBaseURL is the built-in registry container's own
// loopback dial address, mirroring cmd/levelrail's registryDialAddr:
// this runs in the control plane's own process, so there is no reason to
// go through ingress/TLS for a server-side catalog query.
func registryCatalogBaseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", registryreconcile.HostPort)
}

type registryRepositoriesResponse struct {
	Repositories []string `json:"repositories"`
}

type registryTagsResponse struct {
	Repository string   `json:"repository"`
	Tags       []string `json:"tags"`
}

// registryCatalogPassword resolves the built-in registry's generated
// password, the same lookup registryreconcile.Controller.Reconcile makes
// before starting the container. secrets.ErrValueNotFound (no
// credentials generated yet) is a known, non-error absence, the same
// shape credentialsExist establishes there; callers distinguish it from
// a genuine failure via errors.Is.
func (rt *Router) registryCatalogPassword(ctx context.Context) (string, error) {
	return rt.registryCatalogSecrets.Resolve(ctx, store.RegistrySettingsSecretsKey(), store.RegistryPasswordEnvKey)
}

// handleListRegistryRepositories handles GET
// /api/v1/registry/repositories: every repository name pushed to the
// built-in registry, for the "existing image" app-creation step's
// repository picker (CreateAppFields.tsx / GitBuildSourceFields.tsx).
// AbilityRead: no secret is ever returned in the response, only used
// internally to authenticate the upstream query.
func (rt *Router) handleListRegistryRepositories(w http.ResponseWriter, r *http.Request) {
	settings, ok := rt.registryCatalogUsable(w, r)
	if !ok {
		return
	}

	password, err := rt.registryCatalogPassword(r.Context())
	if errors.Is(err, secrets.ErrValueNotFound) {
		writeError(w, http.StatusConflict, "the built-in registry has no credentials generated yet")
		return
	}
	if err != nil {
		rt.logger.Error("api: resolve registry catalog password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	repos, err := rt.registryCatalog.ListRepositories(r.Context(), registryCatalogBaseURL(), settings.Username, password)
	if err != nil {
		rt.logger.Error("api: list registry repositories failed", slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not reach the built-in registry")
		return
	}
	writeJSON(w, http.StatusOK, registryRepositoriesResponse{Repositories: repos})
}

// handleListRegistryTags handles GET
// /api/v1/registry/tags?repository=<name>: every tag pushed for one
// repository. repository is a query parameter, not a {name} path
// segment: Docker repository names routinely contain "/" (a namespaced
// name like "org/app"), which Go's stdlib http.ServeMux path patterns
// can't capture as a single non-trailing parameter, so this stays a
// query string the same way GET /api/v1/apps/{name}/images stays a
// simple path (an app name never contains "/") while this one can't.
func (rt *Router) handleListRegistryTags(w http.ResponseWriter, r *http.Request) {
	repository := r.URL.Query().Get("repository")
	if repository == "" {
		writeError(w, http.StatusBadRequest, "repository query parameter is required")
		return
	}

	settings, ok := rt.registryCatalogUsable(w, r)
	if !ok {
		return
	}

	password, err := rt.registryCatalogPassword(r.Context())
	if errors.Is(err, secrets.ErrValueNotFound) {
		writeError(w, http.StatusConflict, "the built-in registry has no credentials generated yet")
		return
	}
	if err != nil {
		rt.logger.Error("api: resolve registry catalog password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tags, err := rt.registryCatalog.ListTags(r.Context(), registryCatalogBaseURL(), settings.Username, password, repository)
	if errors.Is(err, registrycatalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "repository not found in the built-in registry")
		return
	}
	if err != nil {
		rt.logger.Error("api: list registry tags failed", slog.String("error", err.Error()), slog.String("repository", repository))
		writeError(w, http.StatusBadGateway, "could not reach the built-in registry")
		return
	}
	writeJSON(w, http.StatusOK, registryTagsResponse{Repository: repository, Tags: tags})
}

// registryCatalogUsable is the shared gate both catalog handlers open
// with: 501 when this control plane wasn't wired with a catalog client
// or secrets access (no master key), 409 when the built-in registry
// itself is simply not enabled. Distinct status codes for a wiring gap
// versus an operator's own configuration choice, the same split
// absentResult (internal/reconcile/registry/controller.go) already draws
// for the reconciler's own condition.
func (rt *Router) registryCatalogUsable(w http.ResponseWriter, r *http.Request) (store.RegistrySettings, bool) {
	if rt.registryCatalog == nil || rt.registryCatalogSecrets == nil {
		writeError(w, http.StatusNotImplemented, "the built-in registry is not configured on this control plane (no master key set)")
		return store.RegistrySettings{}, false
	}

	settings, err := rt.registry.GetRegistrySettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get registry settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return store.RegistrySettings{}, false
	}
	if !settings.Enabled {
		writeError(w, http.StatusConflict, "the built-in registry is not enabled")
		return store.RegistrySettings{}, false
	}
	return settings, true
}
