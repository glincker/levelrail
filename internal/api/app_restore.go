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
	st := rt.recentlyDeleted()
	if st == nil {
		return false
	}
	_, err := st.GetRecentlyDeleted(ctx, store.RecentlyDeletedApp, name)
	return err == nil
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
	if scope, _ := callerTeamScope(r.Context()); scope != nil {
		owner := store.ResourceOwner{TeamID: store.DefaultTeamID}
		if svc.ProjectID != "" {
			o, err := rt.ownerOfObject(r.Context(), "project", svc.ProjectID)
			if err != nil && !errors.Is(err, store.ErrProjectNotFound) {
				rt.internalError(w, "api: restore app: resolve team", err, slog.String("name", e.Name))
				return false
			}
			owner = o
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
	}
	// The route gate is not resource scoped, a deleted app has no environment, so honor a deny policy here.
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		rt.internalError(w, "api: restore app: load policies", err)
		return false
	}
	if !authorizeResource(abilities, policies, AbilityWrite, "app:"+e.Name) {
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
