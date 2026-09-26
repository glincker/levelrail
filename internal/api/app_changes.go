package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// changeAggregator builds the change aggregator over whichever sources this
// router has; a missing source is skipped rather than failing the request.
func (rt *Router) changeAggregator() *changes.Aggregator {
	var (
		ev changes.EventSource
		dp changes.DeploySource
		au changes.AuditSource
	)
	if s := rt.appEvents(); s != nil {
		ev = s
	}
	if rt.deployAttempts != nil {
		dp = rt.deployAttempts
	}
	if rt.auditLog != nil {
		au = rt.auditLog
	}
	return changes.New(ev, dp, au, rt.logger)
}

// handleAppChanges handles GET /api/v1/apps/{name}/changes: what changed on
// the app in the window ending at ?until (default now), with the likely cause
// flagged. ?window overrides APP_ALERT_CHANGE_WINDOW.
func (rt *Router) handleAppChanges(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: changes: load app failed", err, slog.String("name", name))
		return
	}
	until := time.Now()
	if v := r.URL.Query().Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "until must be an RFC 3339 timestamp")
			return
		}
		until = t
	}
	agg := rt.changeAggregator()
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 || d > 24*time.Hour {
			writeError(w, http.StatusBadRequest, "window must be a duration between 1s and 24h")
			return
		}
		agg.Window = d
	}
	writeJSON(w, http.StatusOK, agg.Collect(r.Context(), name, until))
}
