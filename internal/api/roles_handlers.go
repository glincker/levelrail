package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/store"
)

// handleListRoles handles GET /api/v1/roles: every stored role with its user count, AbilityRead-gated and never behind the feature flag.
func (rt *Router) handleListRoles(w http.ResponseWriter, r *http.Request) {
	stored, err := rt.roles.ListRoles(r.Context())
	if err != nil {
		rt.logger.Error("api: list roles failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	counts, err := rt.roles.RoleUserCounts(r.Context())
	if err != nil {
		rt.logger.Error("api: count role users failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]Role, 0, len(stored))
	for _, role := range stored {
		out = append(out, toRoleResource(role, counts[role.ID]))
	}
	writeJSON(w, http.StatusOK, out)
}

type roleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Abilities   []string `json:"abilities"`
	Visibility  string   `json:"visibility"`
}

func (rt *Router) writeRoleStoreError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrRoleNotFound):
		writeError(w, http.StatusNotFound, "role not found")
	case errors.Is(err, store.ErrRoleNameTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrRoleBuiltin):
		writeError(w, http.StatusForbidden, "built-in roles cannot be changed")
	case errors.Is(err, store.ErrRoleInUse):
		writeError(w, http.StatusConflict, "the role is assigned to users: reassign them first")
	default:
		rt.logger.Error("api: "+op+" failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// handleCreateRole handles POST /api/v1/roles: AbilityRoot, behind the access-roles flag.
func (rt *Router) handleCreateRole(w http.ResponseWriter, r *http.Request) {
	if !requireAccessRolesFeature(w) {
		return
	}
	var req roleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, visibility, err := validateRoleInput(req.Name, req.Visibility, req.Abilities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := randomOpaqueID("role_")
	if err != nil {
		rt.logger.Error("api: create role: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	role := store.Role{ID: id, Name: name, Description: req.Description, Abilities: req.Abilities, Visibility: visibility}
	if err := rt.roles.CreateRole(r.Context(), role); err != nil {
		rt.writeRoleStoreError(w, "create role", err)
		return
	}
	created, err := rt.roles.GetRole(r.Context(), id)
	if err != nil {
		rt.writeRoleStoreError(w, "create role reload", err)
		return
	}
	writeJSON(w, http.StatusCreated, toRoleResource(created, 0))
}

// handleUpdateRole handles PUT /api/v1/roles/{id}: rewrites a custom role and re-syncs the abilities of its users.
func (rt *Router) handleUpdateRole(w http.ResponseWriter, r *http.Request) {
	if !requireAccessRolesFeature(w) {
		return
	}
	id := r.PathValue("id")
	var req roleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, visibility, err := validateRoleInput(req.Name, req.Visibility, req.Abilities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cur, err := rt.roles.GetRole(r.Context(), id)
	if err != nil {
		rt.writeRoleStoreError(w, "update role", err)
		return
	}
	if cur.Builtin {
		rt.writeRoleStoreError(w, "update role", store.ErrRoleBuiltin)
		return
	}
	if slices.Contains(cur.Abilities, AbilityRoot) && !slices.Contains(req.Abilities, AbilityRoot) {
		outside, err := rt.roles.CountRootUsersOutsideRole(r.Context(), id)
		if err != nil {
			rt.writeRoleStoreError(w, "update role", err)
			return
		}
		if outside == 0 {
			writeError(w, http.StatusConflict, "cannot remove root from the last remaining root user")
			return
		}
	}
	role := store.Role{ID: id, Name: name, Description: req.Description, Abilities: req.Abilities, Visibility: visibility}
	if err := rt.roles.UpdateRole(r.Context(), role); err != nil {
		rt.writeRoleStoreError(w, "update role", err)
		return
	}
	if err := rt.roles.SyncRoleAbilities(r.Context(), id); err != nil {
		rt.writeRoleStoreError(w, "sync role abilities", err)
		return
	}
	updated, err := rt.roles.GetRole(r.Context(), id)
	if err != nil {
		rt.writeRoleStoreError(w, "update role reload", err)
		return
	}
	counts, err := rt.roles.RoleUserCounts(r.Context())
	if err != nil {
		rt.writeRoleStoreError(w, "update role counts", err)
		return
	}
	writeJSON(w, http.StatusOK, toRoleResource(updated, counts[id]))
}

// handleDeleteRole handles DELETE /api/v1/roles/{id}: a built-in or in-use role is refused.
func (rt *Router) handleDeleteRole(w http.ResponseWriter, r *http.Request) {
	if !requireAccessRolesFeature(w) {
		return
	}
	if err := rt.roles.DeleteRole(r.Context(), r.PathValue("id")); err != nil {
		rt.writeRoleStoreError(w, "delete role", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setUserRoleRequest struct {
	RoleID string `json:"role_id"`
}

// handleSetUserRole handles PUT /api/v1/users/{id}/role: assigns a stored role and copies its abilities onto the user.
func (rt *Router) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	if !requireAccessRolesFeature(w) {
		return
	}
	id := r.PathValue("id")
	if callerID, ok := rt.currentSessionUserID(r); ok && callerID == id {
		writeError(w, http.StatusBadRequest, "you cannot change your own role: ask another root user")
		return
	}
	var req setUserRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RoleID == "" {
		writeError(w, http.StatusBadRequest, "role_id is required")
		return
	}
	target, err := rt.auth.GetUserByID(r.Context(), id)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set user role: load user failed", slog.String("user_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	role, err := rt.roles.GetRole(r.Context(), req.RoleID)
	if err != nil {
		rt.writeRoleStoreError(w, "set user role", err)
		return
	}
	if slices.Contains(target.Abilities, AbilityRoot) && !slices.Contains(role.Abilities, AbilityRoot) {
		others, err := rt.roles.CountRootUsersExcept(r.Context(), id)
		if err != nil {
			rt.writeRoleStoreError(w, "set user role", err)
			return
		}
		if others == 0 {
			writeError(w, http.StatusConflict, "cannot remove root from the last remaining root user")
			return
		}
	}
	if err := rt.roles.SetUserRole(r.Context(), id, role.ID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		rt.writeRoleStoreError(w, "set user role", err)
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: set user role: reload failed", slog.String("user_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.userResourceFor(r.Context(), *user, nil))
}

type environmentGrantsBody struct {
	EnvironmentIDs []string `json:"environment_ids"`
}

// handleGetEnvironmentGrants handles GET /api/v1/users/{id}/environment-grants.
func (rt *Router) handleGetEnvironmentGrants(w http.ResponseWriter, r *http.Request) {
	if !requireAccessRolesFeature(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := rt.auth.GetUserByID(r.Context(), id); errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ids, err := rt.roles.ListUserEnvironmentGrants(r.Context(), id)
	if err != nil {
		rt.writeRoleStoreError(w, "list environment grants", err)
		return
	}
	writeJSON(w, http.StatusOK, environmentGrantsBody{EnvironmentIDs: nonNilStrings(ids)})
}

// handlePutEnvironmentGrants handles PUT /api/v1/users/{id}/environment-grants: a full replace, only for users whose role has granted visibility.
func (rt *Router) handlePutEnvironmentGrants(w http.ResponseWriter, r *http.Request) {
	if !requireAccessRolesFeature(w) {
		return
	}
	id := r.PathValue("id")
	var req environmentGrantsBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := rt.auth.GetUserByID(r.Context(), id); errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	roleID, err := rt.roles.UserRoleID(r.Context(), id)
	if err != nil {
		rt.writeRoleStoreError(w, "environment grants", err)
		return
	}
	role, err := rt.roles.GetRole(r.Context(), roleID)
	if err != nil || role.Visibility != store.RoleVisibilityGranted {
		writeError(w, http.StatusBadRequest, "the user's role does not use environment grants")
		return
	}
	ids := make([]string, 0, len(req.EnvironmentIDs))
	for _, envID := range req.EnvironmentIDs {
		if slices.Contains(ids, envID) {
			continue
		}
		if _, err := rt.environments.GetEnvironment(r.Context(), envID); err != nil {
			writeError(w, http.StatusBadRequest, "unknown environment: "+envID)
			return
		}
		ids = append(ids, envID)
	}
	if err := rt.roles.SetUserEnvironmentGrants(r.Context(), id, ids); err != nil {
		rt.writeRoleStoreError(w, "set environment grants", err)
		return
	}
	writeJSON(w, http.StatusOK, environmentGrantsBody{EnvironmentIDs: ids})
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
