package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

const buildSpeedWindow = 50

// handleBuildSpeed handles GET /api/v1/apps/{name}/build-speed: time saved by cache and typed speedup fixes.
func (rt *Router) handleBuildSpeed(w http.ResponseWriter, r *http.Request) {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "build stats are not available on this control plane")
		return
	}
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: build speed: load app failed", err)
		return
	}
	rows, err := rt.buildPerf.ListBuildRuns(r.Context(), name, buildSpeedWindow)
	if err != nil {
		rt.internalError(w, "api: build speed: list runs failed", err)
		return
	}
	runs := make([]build.Run, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, build.Run{
			Status: build.RunStatus(row.Status), Duration: time.Duration(row.DurationMS) * time.Millisecond,
			CachedSteps: row.CachedSteps, TotalSteps: row.TotalSteps,
		})
	}
	writeJSON(w, http.StatusOK, build.AdviseSpeed(runs, rt.buildCacheState(r, name)))
}

func (rt *Router) buildCacheState(r *http.Request, app string) build.SpeedCacheState {
	if rt.storage == nil || rt.storage.BuildCacheSettings == nil {
		return build.SpeedCacheState{}
	}
	for _, scope := range []string{app, ""} {
		s, err := rt.storage.BuildCacheSettings.GetBuildCacheSetting(r.Context(), scope)
		if err == nil {
			return build.SpeedCacheState{Configured: true, Enabled: s.Enabled, TargetID: s.TargetID}
		}
	}
	return build.SpeedCacheState{}
}
