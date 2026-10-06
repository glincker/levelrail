package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxEnvironmentNameLen = 64

var reservedEnvironmentIDs = map[string]bool{
	"env_dev": true, "env_test": true, "env_uat": true, "env_production": true,
}

var assignableEnvironmentKinds = map[string]bool{
	store.EnvironmentKindDev: true, store.EnvironmentKindTest: true, store.EnvironmentKindUAT: true,
	store.EnvironmentKindProduction: true, store.EnvironmentKindCustom: true,
}

const environmentKindsMessage = "kind must be one of dev, test, uat, production, custom"

// environmentListResource is one row of GET /api/v1/environments.
type environmentListResource struct {
	environmentResource
	AppCount      int `json:"app_count"`
	DatabaseCount int `json:"database_count"`
}

// requireExperimentalOn writes the standard disabled response and returns
// false when f is off.
func requireExperimentalOn(w http.ResponseWriter, f experimental.Feature) bool {
	if experimental.Enabled(f) {
		return true
	}
	writeJSON(w, http.StatusNotFound, experimentalError{
		Error: experimental.DisabledMessage(f), Code: ExperimentalDisabledCode, Feature: string(f),
	})
	return false
}

func validateEnvironmentName(name string) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "name is required"
	case len(name) > maxEnvironmentNameLen:
		return fmt.Sprintf("name must be at most %d characters", maxEnvironmentNameLen)
	}
	return ""
}

func validateEnvironmentPatch(current store.Environment, req updateEnvironmentRequest) string {
	if req.Name != nil {
		if msg := validateEnvironmentName(*req.Name); msg != "" {
			return msg
		}
	}
	if req.Kind != nil {
		if !assignableEnvironmentKinds[*req.Kind] {
			return environmentKindsMessage
		}
		if *req.Kind != current.Kind && (reservedEnvironmentIDs[current.ID] || current.Kind == store.EnvironmentKindPreview) {
			return "the kind of a built-in or preview environment cannot change"
		}
	}
	return ""
}

// handleListAllEnvironments handles GET /api/v1/environments.
func (rt *Router) handleListAllEnvironments(w http.ResponseWriter, r *http.Request) {
	envs, err := rt.environments.ListAllEnvironments(r.Context())
	if err != nil {
		rt.internalError(w, "api: list all environments failed", err)
		return
	}
	out := make([]environmentListResource, 0, len(envs))
	for _, e := range envs {
		out = append(out, environmentListResource{environmentResource: toEnvironmentResource(e.Environment), AppCount: e.AppCount, DatabaseCount: e.DatabaseCount})
	}
	writeJSON(w, http.StatusOK, out)
}

type createGlobalEnvironmentRequest struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Protected bool   `json:"protected,omitempty"`
	SortOrder int    `json:"sort_order,omitempty"`
}

// handleCreateGlobalEnvironment handles POST /api/v1/environments.
func (rt *Router) handleCreateGlobalEnvironment(w http.ResponseWriter, r *http.Request) {
	var req createGlobalEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateEnvironmentName(req.Name); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.Kind == "" {
		req.Kind = store.EnvironmentKindCustom
	}
	if !assignableEnvironmentKinds[req.Kind] {
		writeError(w, http.StatusBadRequest, environmentKindsMessage)
		return
	}
	id, err := randomEnvironmentID()
	if err != nil {
		rt.internalError(w, "api: create global environment: generate id failed", err)
		return
	}
	e := store.Environment{
		ID: id, ProjectID: store.GlobalProjectID, Name: strings.TrimSpace(req.Name), Protected: req.Protected,
		CreatedAt: time.Now().UTC().Format(time.RFC3339), Kind: req.Kind, Scope: store.EnvironmentScopeGlobal, SortOrder: req.SortOrder,
	}
	if err := rt.environments.SaveEnvironment(r.Context(), e); err != nil {
		rt.internalError(w, "api: create global environment failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, toEnvironmentResource(e))
}

// refuseDeleteWithMembers writes 409 and returns false when a global
// environment still has apps or databases tagged.
func (rt *Router) refuseDeleteWithMembers(w http.ResponseWriter, r *http.Request, id string) bool {
	e, err := rt.environments.GetEnvironment(r.Context(), id)
	if err != nil || e.Scope != store.EnvironmentScopeGlobal {
		return true
	}
	apps, dbs, err := rt.environments.EnvironmentMemberCounts(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: delete environment: count members failed", err)
		return false
	}
	if apps+dbs > 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("environment %q still has %d app(s) and %d database(s); pass move_to=<environment id> to move them first", e.Name, apps, dbs))
		return false
	}
	return true
}

// moveMembersBeforeDelete retags every member of id to target ahead of a
// delete. A protected source or target is refused: a bulk retag would skip
// the per-resource approval a protected move needs.
func (rt *Router) moveMembersBeforeDelete(w http.ResponseWriter, r *http.Request, id, target string) bool {
	if id == target {
		writeError(w, http.StatusBadRequest, "move_to must be a different environment")
		return false
	}
	src, err := rt.environments.GetEnvironment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "environment not found")
		return false
	}
	dst, err := rt.environments.GetEnvironment(r.Context(), target)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unknown move_to environment")
		return false
	}
	if src.Protected || dst.Protected {
		writeError(w, http.StatusConflict, "a protected environment cannot take part in a bulk move; unprotect it or move apps one at a time with approval")
		return false
	}
	if err := rt.environments.MoveEnvironmentMembers(r.Context(), id, target); err != nil {
		rt.internalError(w, "api: delete environment: move members failed", err)
		return false
	}
	return true
}
