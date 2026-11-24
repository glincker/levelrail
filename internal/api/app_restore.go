package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// deletedNameReserved reports whether name is held by an app inside its undo window.
func (rt *Router) deletedNameReserved(ctx context.Context, name string) bool {
	if rt.recentlyDeleted() == nil {
		return false
	}
	_, err := rt.liveDeleted(ctx, store.RecentlyDeletedApp, name)
	// A store error keeps the name reserved: reusing it could hand over the kept history.
	return !errors.Is(err, store.ErrRecentlyDeletedNotFound)
}

// deletedAppEnvironment resolves the environment a deleted app was tagged with; nil when none or gone.
func (rt *Router) deletedAppEnvironment(ctx context.Context, svc store.DesiredService) (*store.EnvironmentRef, error) {
	if svc.EnvironmentID == "" {
		return nil, nil
	}
	var getter interface {
		GetEnvironment(ctx context.Context, id string) (store.Environment, error)
	}
	if g, ok := rt.policies.(interface {
		GetEnvironment(ctx context.Context, id string) (store.Environment, error)
	}); ok {
		getter = g
	} else if g, ok := rt.apps.(interface {
		GetEnvironment(ctx context.Context, id string) (store.Environment, error)
	}); ok {
		getter = g
	}
	if getter == nil {
		return nil, nil
	}
	env, err := getter.GetEnvironment(ctx, svc.EnvironmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &store.EnvironmentRef{ID: env.ID, Name: env.Name, Kind: env.Kind, Scope: env.Scope, ProjectID: env.ProjectID, Protected: env.Protected}, nil
}

// deletedAppOwner is the owner of a deleted app's project, the default team when it had none.
func (rt *Router) deletedAppOwner(ctx context.Context, svc store.DesiredService) (store.ResourceOwner, error) {
	owner := store.ResourceOwner{TeamID: store.DefaultTeamID}
	if svc.ProjectID == "" {
		return owner, nil
	}
	o, err := rt.ownerOfObject(ctx, "project", svc.ProjectID)
	if err != nil && !errors.Is(err, store.ErrProjectNotFound) {
		return owner, err
	}
	return o, nil
}

// deletedVisibleToCaller hides another team's undo entries and, for a restricted caller, other environments.
// A lookup failure hides the row.
func (rt *Router) deletedVisibleToCaller(r *http.Request, e store.RecentlyDeleted) bool {
	scope, _ := callerTeamScope(r.Context())
	if e.Kind != store.RecentlyDeletedApp {
		return scope == nil || scope.instance
	}
	if scope == nil {
		return true
	}
	var svc store.DesiredService
	if err := json.Unmarshal([]byte(e.Snapshot), &svc); err != nil {
		return false
	}
	owner, err := rt.deletedAppOwner(r.Context(), svc)
	if err != nil {
		rt.logger.Warn("api: recently deleted: resolve team failed", slog.String("name", e.Name), slog.String("error", err.Error()))
		return false
	}
	return scope.roleFor(&owner) != ""
}

// authorizeAppRestore runs the checks a restore adds to the route's ability gate: the original team,
// the typed name and that every volume is still there. It writes the refusal itself.
func (rt *Router) authorizeAppRestore(w http.ResponseWriter, r *http.Request, e store.RecentlyDeleted) bool {
	var svc store.DesiredService
	if err := json.Unmarshal([]byte(e.Snapshot), &svc); err != nil {
		rt.internalError(w, "api: restore app: read snapshot", err, slog.String("name", e.Name))
		return false
	}
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: restore app: resolve caller", err)
		return false
	}
	ref, err := rt.deletedAppEnvironment(r.Context(), svc)
	if err != nil {
		rt.internalError(w, "api: restore app: resolve environment", err, slog.String("name", e.Name))
		return false
	}
	vis, err := rt.principalVisibility(r.Context(), principalType, principalID)
	if err != nil {
		rt.internalError(w, "api: restore app: resolve visibility", err)
		return false
	}
	if vis.Restricted && (ref == nil || !vis.Allows(ref.ID)) {
		writeError(w, http.StatusNotFound, "nothing to restore: the undo window has passed or it was never deleted")
		return false
	}
	if scope, _ := callerTeamScope(r.Context()); scope != nil {
		owner, err := rt.deletedAppOwner(r.Context(), svc)
		if err != nil {
			rt.internalError(w, "api: restore app: resolve team", err, slog.String("name", e.Name))
			return false
		}
		role := scope.roleFor(&owner)
		if role == "" {
			writeError(w, http.StatusNotFound, "nothing to restore: the undo window has passed or it was never deleted")
			return false
		}
		if !hasAbility(capAbilities(abilities, owner.TeamID, role), AbilityWrite) {
			writeError(w, http.StatusForbidden, msgTeamRoleTooLow)
			return false
		}
		if protectedDenied(role, AbilityWrite, ref) {
			writeError(w, http.StatusForbidden, "this environment is protected, restoring needs an admin or owner")
			return false
		}
	}
	// The route gate is not resource scoped and the app is gone, so apply its snapshot's environment here.
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		rt.internalError(w, "api: restore app: load policies", err)
		return false
	}
	if !authorizeResource(abilities, policies, AbilityWrite, "app:"+e.Name, environmentResources(ref)...) {
		writeError(w, http.StatusForbidden, "a policy denies restoring this app")
		return false
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || body.Confirm != e.Name {
		writeError(w, http.StatusBadRequest, "restore needs confirm set to the exact app name")
		return false
	}
	if missing := rt.missingRestoreVolume(r.Context(), svc); missing != "" {
		writeError(w, http.StatusConflict, fmt.Sprintf("volume %q is gone from its node, so the app cannot be restored with its data; restore the volume from a backup first", missing))
		return false
	}
	return true
}

// existingVolume names the first of svc's volumes already on its node, confirmed present; "" when none or unknown.
// A volume left by a deleted app of the same name would otherwise be mounted by the next one.
func (rt *Router) existingVolume(ctx context.Context, svc store.DesiredService) string {
	if rt.backupProtection == nil || rt.execRuntime == nil {
		return ""
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return ""
	}
	for _, v := range svc.Volumes {
		if v.Name == "" {
			continue
		}
		if exists, known, err := rt.backupProtection.VolumeExists(ctx, nodeRuntime, v.Name); err == nil && known && exists {
			return v.Name
		}
	}
	return ""
}

// missingRestoreVolume names the first volume confirmed absent. A node that cannot answer is not a refusal.
func (rt *Router) missingRestoreVolume(ctx context.Context, svc store.DesiredService) string {
	if rt.backupProtection == nil || rt.execRuntime == nil {
		return ""
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return ""
	}
	for _, v := range svc.Volumes {
		if v.Name == "" || v.Name == deploy.NextImageCacheVolume(svc.Name) {
			continue
		}
		exists, known, err := rt.backupProtection.VolumeExists(ctx, nodeRuntime, v.Name)
		if err == nil && known && !exists {
			return v.Name
		}
	}
	return ""
}

// authorizeDatabaseRestore keeps database restores instance wide and honors a deny policy.
func (rt *Router) authorizeDatabaseRestore(w http.ResponseWriter, r *http.Request, name string) bool {
	if scope, _ := callerTeamScope(r.Context()); scope != nil && !scope.instance {
		writeError(w, http.StatusForbidden, msgInstanceOnly)
		return false
	}
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: restore database: resolve caller", err)
		return false
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		rt.internalError(w, "api: restore database: load policies", err)
		return false
	}
	if !authorizeResource(abilities, policies, AbilityWrite, "database:"+name) {
		writeError(w, http.StatusForbidden, "a policy denies restoring this database")
		return false
	}
	return true
}
