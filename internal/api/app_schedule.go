package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

const (
	defaultAppScheduleHistoryLimit = 20
	maxAppScheduleHistoryLimit     = 100
)

// AppScheduleStore is the store surface the schedule handlers and
// internal/scheduledeploy.Scheduler both need. *store.DB satisfies this
// structurally.
type AppScheduleStore interface {
	GetAppSchedule(ctx context.Context, serviceName string) (*store.AppSchedule, error)
	SetAppSchedule(ctx context.Context, serviceName, cron, branch, timezone string, enabled bool) error
	ArmAppScheduleNextRun(ctx context.Context, serviceName string, next time.Time) error
	ListAppScheduleHistory(ctx context.Context, serviceName string, limit int) ([]store.AppScheduleHistoryEntry, error)
}

// appScheduleResource is the wire shape for GET/PUT .../schedule.
type appScheduleResource struct {
	ServiceName string     `json:"service_name"`
	Cron        string     `json:"cron"`
	Branch      string     `json:"branch"`
	Timezone    string     `json:"timezone"`
	Enabled     bool       `json:"enabled"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
}

func toAppScheduleResource(s store.AppSchedule) appScheduleResource {
	return appScheduleResource{
		ServiceName: s.ServiceName, Cron: s.Cron, Branch: s.Branch, Timezone: s.Timezone,
		Enabled: s.Enabled, NextRunAt: s.NextFireAt,
	}
}

// handleGetAppSchedule handles GET /api/v1/apps/{name}/schedule. No
// schedule configured is a normal state (404 with a plain body), the
// same "absence is not an error condition, just an empty result" shape
// handleGetVolumeBackupSchedule's own doc comment establishes for a
// sibling per-app optional setting.
func (rt *Router) handleGetAppSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	sch, err := rt.appSchedules.GetAppSchedule(r.Context(), name)
	if errors.Is(err, store.ErrAppScheduleNotFound) {
		writeError(w, http.StatusNotFound, "no schedule configured for this app")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app schedule failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAppScheduleResource(*sch))
}

type setAppScheduleRequest struct {
	Cron     string `json:"cron"`
	Branch   string `json:"branch"`
	Timezone string `json:"timezone"`
	Enabled  *bool  `json:"enabled"`
}

// handleSetAppSchedule handles PUT /api/v1/apps/{name}/schedule: sets or
// replaces the app's recurring redeploy schedule. Requires a connected
// git source (internal/scheduledeploy has nothing to redeploy otherwise)
// and a syntactically valid cron expression, both checked synchronously
// before any state is written, the same "validate first" shape
// validateBackupScheduleRequest already establishes for a sibling
// schedule setting. enabled defaults to true when omitted, so the
// smallest useful request body is {"cron": "...", "branch": "..."}.
func (rt *Router) handleSetAppSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app schedule: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := rt.gitSources.GetGitSource(r.Context(), name); errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusBadRequest, "this app has no connected git source; connect one via PUT /api/v1/apps/{name}/git-source before scheduling deploys")
		return
	} else if err != nil {
		rt.logger.Error("api: set app schedule: load git source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setAppScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Branch = strings.TrimSpace(req.Branch); req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch is required")
		return
	}
	if req.Cron == "" {
		writeError(w, http.StatusBadRequest, "cron is required")
		return
	}
	sched, err := cronexpr.Parse(req.Cron)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid cron expression: %s", err.Error()))
		return
	}
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid timezone: %s", err.Error()))
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := rt.appSchedules.SetAppSchedule(r.Context(), name, req.Cron, req.Branch, timezone, enabled); err != nil {
		rt.logger.Error("api: set app schedule failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resource := appScheduleResource{ServiceName: name, Cron: req.Cron, Branch: req.Branch, Timezone: timezone, Enabled: enabled}
	// Arm the next occurrence immediately rather than leaving next_fire_at
	// NULL until the scheduler's own next tick: otherwise a client that
	// just saved a schedule sees no next-run time at all until that tick
	// happens, which can be minutes away. This mirrors exactly what the
	// scheduler itself would compute on first sight of the schedule
	// (internal/scheduledeploy.Scheduler.tickOne), so it changes nothing
	// about when the schedule actually fires.
	if enabled {
		next := cronexpr.NextInLocation(sched, time.Now(), loc)
		if err := rt.appSchedules.ArmAppScheduleNextRun(r.Context(), name, next); err != nil {
			rt.logger.Error("api: arm app schedule next run failed", slog.String("error", err.Error()), slog.String("name", name))
		} else {
			resource.NextRunAt = &next
		}
	}

	writeJSON(w, http.StatusOK, resource)
}

type appScheduleHistoryResource struct {
	ID           string    `json:"id"`
	ScheduledFor time.Time `json:"scheduled_for"`
	FiredAt      time.Time `json:"fired_at"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
}

// handleListAppScheduleHistory handles
// GET /api/v1/apps/{name}/schedule/history: the most recent evaluations
// of this app's schedule, newest first, whether they fired, were skipped
// for an active freeze window, or failed.
func (rt *Router) handleListAppScheduleHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	limit := defaultAppScheduleHistoryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxAppScheduleHistoryLimit {
		limit = maxAppScheduleHistoryLimit
	}

	entries, err := rt.appSchedules.ListAppScheduleHistory(r.Context(), name, limit)
	if err != nil {
		rt.logger.Error("api: list app schedule history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]appScheduleHistoryResource, 0, len(entries))
	for _, e := range entries {
		out = append(out, appScheduleHistoryResource{ID: e.ID, ScheduledFor: e.ScheduledFor, FiredAt: e.FiredAt, Status: e.Status, Reason: e.Reason})
	}
	writeJSON(w, http.StatusOK, out)
}
