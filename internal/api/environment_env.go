package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleGetEnvironmentEnv handles GET /api/v1/environments/{id}/env.
func (rt *Router) handleGetEnvironmentEnv(w http.ResponseWriter, r *http.Request) {
	rt.handleGetSharedEnv(w, r, rt.environmentEnvScope())
}

// handleSetEnvironmentEnv handles PUT /api/v1/environments/{id}/env: full
// replace, mirroring handleSetProjectEnv/handleSetOrganizationEnv.
func (rt *Router) handleSetEnvironmentEnv(w http.ResponseWriter, r *http.Request) {
	rt.handleSetSharedEnv(w, r, rt.environmentEnvScope())
}

func (rt *Router) environmentEnvScope() sharedEnvScope {
	return sharedEnvScope{
		label:       "environment",
		notFoundMsg: "environment not found",
		notFound:    store.ErrEnvironmentNotFound,
		load: func(ctx context.Context, id string) error {
			_, err := rt.environments.GetEnvironment(ctx, id)
			return err
		},
		list: rt.environments.ListEnvironmentEnvVars,
		set:  rt.environments.SetEnvironmentEnvVars,
	}
}

func (rt *Router) environmentEnvSecretScope() sharedEnvSecretScope {
	return sharedEnvSecretScope{
		label:       "environment",
		notFoundMsg: "environment not found",
		notFound:    store.ErrEnvironmentNotFound,
		load: func(ctx context.Context, id string) error {
			_, err := rt.environments.GetEnvironment(ctx, id)
			return err
		},
		listAll:    rt.environments.ListEnvironmentEnvVarsDetailed,
		listKeys:   rt.environments.ListEnvironmentSecretEnvKeys,
		markSecret: rt.environments.SetEnvironmentSecretEnvVar,
		unmark:     rt.environments.DeleteEnvironmentSecretEnvVar,
		namespace:  store.EnvironmentEnvSecretsKey,
	}
}

// handleListEnvironmentEnvAll handles GET
// /api/v1/environments/{id}/env/all.
func (rt *Router) handleListEnvironmentEnvAll(w http.ResponseWriter, r *http.Request) {
	rt.handleListSharedEnvAll(w, r, rt.environmentEnvSecretScope())
}

// handleListEnvironmentEnvSecretKeys handles GET
// /api/v1/environments/{id}/env/secrets.
func (rt *Router) handleListEnvironmentEnvSecretKeys(w http.ResponseWriter, r *http.Request) {
	rt.handleListSharedEnvSecretKeys(w, r, rt.environmentEnvSecretScope())
}

// handleSetEnvironmentEnvSecret handles PUT
// /api/v1/environments/{id}/env/secrets/{key}.
func (rt *Router) handleSetEnvironmentEnvSecret(w http.ResponseWriter, r *http.Request) {
	rt.handleSetSharedEnvSecret(w, r, rt.environmentEnvSecretScope())
}

// handleDeleteEnvironmentEnvSecret handles DELETE
// /api/v1/environments/{id}/env/secrets/{key}.
func (rt *Router) handleDeleteEnvironmentEnvSecret(w http.ResponseWriter, r *http.Request) {
	rt.handleDeleteSharedEnvSecret(w, r, rt.environmentEnvSecretScope())
}
