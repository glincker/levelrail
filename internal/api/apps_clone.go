package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// cloneAppRequest is POST /api/v1/apps/{name}/clone's body.
type cloneAppRequest struct {
	NewName string `json:"new_name"`
	// CopySecrets re-encrypts the source's secret values under the clone's
	// own slot bindings; the caller must hold read:sensitive.
	CopySecrets bool `json:"copy_secrets,omitempty"`
	// Domains is "none" (default) or "suffix", which derives each clone
	// domain by appending DomainSuffix to the first label.
	Domains      string `json:"domains,omitempty"`
	DomainSuffix string `json:"domain_suffix,omitempty"`
	// EnvironmentID places the clone in an environment of the source's project.
	EnvironmentID string `json:"environment_id,omitempty"`
}

// handleCloneApp duplicates {name}'s desired state under req.NewName, with the same 409 conflict rule as handleCreateApp.
// Domains, secret values and NodeID do not carry over: domains would collide, secret keys are scoped per service, and placement is a deliberate follow-up.
// ProjectID carries over, since a project is a pure organizational label.
func (rt *Router) handleCloneApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req cloneAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.NewName == "" {
		writeError(w, http.StatusBadRequest, "new_name is required")
		return
	}

	if req.CopySecrets && !rt.callerHasAbility(r, AbilityReadSensitive) {
		writeError(w, http.StatusForbidden, "copying secret values requires read:sensitive")
		return
	}
	if req.CopySecrets && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured")
		return
	}
	cloneDomains, err := cloneDomainsFor(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	source, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: clone app: load source failed", err, slog.String("name", name))
		return
	}

	// Held through the row save and secret copy; released before any rollback, which takes the lock itself.
	unlockName := lockAppName(req.NewName)
	defer unlockName()

	// Same existence check as handleCreateApp; it also rejects a self-clone as a conflict instead of overwriting the source.
	_, err = rt.apps.GetDesiredService(r.Context(), req.NewName)
	if err == nil {
		writeError(w, http.StatusConflict, "an app with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: clone app: check new name failed", err, slog.String("new_name", req.NewName))
		return
	}

	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, errCloneEnvironmentMsg), environmentRef(req.EnvironmentID)) {
		return
	}
	if err := rt.validateCloneEnvironment(r.Context(), *source, req.EnvironmentID); err != nil {
		var bad errCloneRequest
		if errors.As(err, &bad) {
			writeError(w, http.StatusBadRequest, bad.Error())
			return
		}
		rt.internalError(w, "api: clone app: check environment failed", err, slog.String("new_name", req.NewName))
		return
	}

	clone := cloneDesiredService(*source, req.NewName, deriveCloneDomains(source.Domains, cloneDomains))
	if err := rt.apps.SaveDesiredService(r.Context(), clone); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.internalError(w, "api: clone app failed", err, slog.String("name", name), slog.String("new_name", req.NewName))
		return
	}

	if source.ProjectID != "" {
		if err := rt.apps.UpdateServiceProject(r.Context(), req.NewName, source.ProjectID); err != nil {
			unlockName()
			rt.rollbackClone(r.Context(), req.NewName)
			rt.internalError(w, "api: clone app: assign project failed", err, slog.String("new_name", req.NewName), slog.String("project_id", source.ProjectID))
			return
		}
	}

	if err := rt.applyCloneExtras(r.Context(), *source, req); err != nil {
		unlockName()
		rt.rollbackClone(r.Context(), req.NewName)
		var bad errCloneRequest
		if errors.As(err, &bad) {
			writeError(w, http.StatusBadRequest, bad.Error())
			return
		}
		rt.internalError(w, "api: clone app: apply options failed", err, slog.String("new_name", req.NewName))
		return
	}

	unlockName()

	cloned, err := rt.apps.GetDesiredService(r.Context(), req.NewName)
	if err != nil {
		rt.internalError(w, "api: clone app: reload after clone failed", err, slog.String("new_name", req.NewName))
		return
	}
	rt.copyEnvSettings(r.Context(), name, req.NewName)
	writeJSON(w, http.StatusCreated, rt.appOut(r.Context(), *cloned))
}
