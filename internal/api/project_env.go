package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// decodeEnvVars decodes r's body as the plain map both the organization-
// and project-level env endpoints use, defaulting a null body to an
// empty map so PUT-ing "null" clears every var rather than erroring.
func decodeEnvVars(r *http.Request) (map[string]string, error) {
	var vars map[string]string
	if err := json.NewDecoder(r.Body).Decode(&vars); err != nil {
		return nil, err
	}
	if vars == nil {
		vars = map[string]string{}
	}
	return vars, nil
}

// handleGetProjectEnv handles GET /api/v1/projects/{id}/env: the shared
// env vars every app filed under this project inherits as its
// resolveEnv base layer (internal/reconcile/application). A plain map,
// the same wire shape appResource.Env already has.
func (rt *Router) handleGetProjectEnv(w http.ResponseWriter, r *http.Request) {
	rt.handleGetSharedEnv(w, r, rt.projectEnvScope())
}

// handleSetProjectEnv handles PUT /api/v1/projects/{id}/env: full
// replace, the same semantics PUT /apps/{name}'s own env field has, not
// a partial patch. See store.DB.SetProjectEnvVars's own doc comment.
func (rt *Router) handleSetProjectEnv(w http.ResponseWriter, r *http.Request) {
	rt.handleSetSharedEnv(w, r, rt.projectEnvScope())
}

func (rt *Router) projectEnvScope() sharedEnvScope {
	return sharedEnvScope{
		label:       "project",
		notFoundMsg: "project not found",
		notFound:    store.ErrProjectNotFound,
		load: func(ctx context.Context, id string) error {
			_, err := rt.projects.GetProject(ctx, id)
			return err
		},
		list: rt.projects.ListProjectEnvVars,
		set:  rt.projects.SetProjectEnvVars,
	}
}
