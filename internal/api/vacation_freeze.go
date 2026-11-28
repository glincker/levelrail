package api

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/deploy"
	"github.com/glincker/levelrail/internal/store"
)

type vacationFreezeResource struct {
	Active   bool       `json:"active"`
	Until    *time.Time `json:"until,omitempty"`
	DaysLeft int        `json:"days_left"`
	Reason   string     `json:"reason,omitempty"`
}

type putVacationFreezeRequest struct {
	Days   int    `json:"days"`
	Reason string `json:"reason,omitempty"`
}

func vacationView(ws []store.DeployFreezeWindow, now time.Time) vacationFreezeResource {
	for _, w := range ws {
		active, end, err := deploy.WindowActive(w, now)
		if err != nil || !active {
			continue
		}
		until := end.UTC()
		return vacationFreezeResource{Active: true, Until: &until, Reason: w.Reason, DaysLeft: int(math.Ceil(end.Sub(now).Hours() / 24))}
	}
	return vacationFreezeResource{}
}

// handleGetVacationFreeze handles GET /api/v1/settings/vacation-freeze.
func (rt *Router) handleGetVacationFreeze(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy freeze windows are not configured on this control plane")
		return
	}
	ws, err := rt.deploySafety.ListDeployFreezeWindows(r.Context(), store.DeployFreezeScopeVacation)
	if err != nil {
		rt.internalError(w, "api: get vacation freeze failed", err)
		return
	}
	writeJSON(w, http.StatusOK, vacationView(ws, time.Now()))
}

// handlePutVacationFreeze handles PUT /api/v1/settings/vacation-freeze: one
// switch that holds auto deploys, scheduled jobs and held-deploy replay for
// days, then lifts itself. A second call restarts the clock.
func (rt *Router) handlePutVacationFreeze(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy freeze windows are not configured on this control plane")
		return
	}
	var req putVacationFreezeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := deploy.ValidateVacationDays(req.Days); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	win := deploy.VacationWindow(now, req.Days, strings.TrimSpace(req.Reason))
	if win.Reason == "" {
		win.Reason = "vacation"
	}
	saved, err := rt.deploySafety.ReplaceDeployFreezeWindows(r.Context(), store.DeployFreezeScopeVacation, []store.DeployFreezeWindow{win})
	if err != nil {
		rt.internalError(w, "api: save vacation freeze failed", err)
		return
	}
	rt.logger.Info("api: vacation freeze on", slog.Int("days", req.Days))
	writeJSON(w, http.StatusOK, vacationView(saved, now))
}

// handleDeleteVacationFreeze handles DELETE /api/v1/settings/vacation-freeze.
func (rt *Router) handleDeleteVacationFreeze(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy freeze windows are not configured on this control plane")
		return
	}
	if _, err := rt.deploySafety.ReplaceDeployFreezeWindows(r.Context(), store.DeployFreezeScopeVacation, nil); err != nil {
		rt.internalError(w, "api: clear vacation freeze failed", err)
		return
	}
	writeJSON(w, http.StatusOK, vacationFreezeResource{})
}
