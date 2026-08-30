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
