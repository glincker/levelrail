package api

import (
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// CrashBreaker is the crash-loop breaker the reconciler consults. Resume clears the
// quarantine of every container of an app and reports how many were held.
type CrashBreaker interface {
	Resume(serviceName string) int
}

// SetCrashBreaker attaches the shared breaker once boot has built it.
func (rt *Router) SetCrashBreaker(b CrashBreaker) {
	rt.crashBreaker.Store(&b)
}

type resumeAppResponse struct {
	Resumed int `json:"resumed"`
}

// handleResumeApp handles POST /api/v1/apps/{name}/resume: lets a quarantined app start again.
func (rt *Router) handleResumeApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: resume app failed", err)
		return
	}
	held := rt.resumeCrashBreaker(name)
	if held > 0 {
		rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventRestart, Title: "Resumed after crash-loop quarantine"})
		rt.nudgeReconciler()
	}
	writeJSON(w, http.StatusOK, resumeAppResponse{Resumed: held})
}

func (rt *Router) resumeCrashBreaker(name string) int {
	if b := rt.crashBreaker.Load(); b != nil {
		return (*b).Resume(name)
	}
	return 0
}
