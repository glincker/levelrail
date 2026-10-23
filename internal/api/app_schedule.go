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
	envAppScheduleRunAtMinLead    = "APP_SCHEDULE_RUN_AT_MIN_LEAD"
	envAppScheduleRunAtMaxHorizon = "APP_SCHEDULE_RUN_AT_MAX_HORIZON"

	defaultAppScheduleRunAtMinLead    = time.Minute
	defaultAppScheduleRunAtMaxHorizon = 365 * 24 * time.Hour

	defaultAppScheduleHistoryLimit = 20
	maxAppScheduleHistoryLimit     = 100
)

// AppScheduleStore is the store surface the schedule handlers and
// internal/scheduledeploy.Scheduler both need. *store.DB satisfies this
// structurally.
type AppScheduleStore interface {
	GetAppSchedule(ctx context.Context, serviceName string) (*store.AppSchedule, error)
	SetAppSchedule(ctx context.Context, serviceName, cron, branch, timezone string, enabled bool) error
	SetAppScheduleRunAt(ctx context.Context, serviceName, branch, timezone string, runAt time.Time, enabled bool) error
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
	// RunAt is set for a one-time deploy (Cron is then empty); CompletedAt
	// is set once that one-time deploy has fired or been skipped.
	RunAt       *time.Time `json:"run_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func toAppScheduleResource(s store.AppSchedule) appScheduleResource {
	return appScheduleResource{
		ServiceName: s.ServiceName, Cron: s.Cron, Branch: s.Branch, Timezone: s.Timezone,
		Enabled: s.Enabled, NextRunAt: s.NextFireAt, RunAt: s.RunAt, CompletedAt: s.CompletedAt,
	}
}

// Returns a 404 with a plain body when no schedule is configured; that is a normal state, not an error.
func (rt *Router) handleGetAppSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	sch, err := rt.appSchedules.GetAppSchedule(r.Context(), name)
	if errors.Is(err, store.ErrAppScheduleNotFound) {
		writeError(w, http.StatusNotFound, "no schedule configured for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get app schedule failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toAppScheduleResource(*sch))
}

type setAppScheduleRequest struct {
	Cron     string `json:"cron"`
	RunAt    string `json:"run_at"`
	Branch   string `json:"branch"`
	Timezone string `json:"timezone"`
	Enabled  *bool  `json:"enabled"`
}

// handleSetAppSchedule sets or replaces the app's redeploy schedule: recurring (cron) or one-time (run_at).
// Requires a connected git source; cron and source are validated before any state is written.
func (rt *Router) handleSetAppSchedule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app schedule: load app failed", err, slog.String("name", name))
		return
	}

	if _, err := rt.gitSources.GetGitSource(r.Context(), name); errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusBadRequest, "this app has no connected git source; connect one via PUT /api/v1/apps/{name}/git-source before scheduling deploys")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app schedule: load git source failed", err, slog.String("name", name))
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
	var sched *cronexpr.Schedule
	var runAt time.Time
	switch {
	case req.Cron != "" && req.RunAt != "":
		writeError(w, http.StatusBadRequest, "set exactly one of cron or run_at, not both")
		return
	case req.Cron == "" && req.RunAt == "":
		writeError(w, http.StatusBadRequest, "one of cron or run_at is required")
		return
	case req.Cron != "":
		var err error
		if sched, err = cronexpr.Parse(req.Cron); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid cron expression: %s", err.Error()))
			return
		}
	default:
		var err error
		if runAt, err = parseAppScheduleRunAt(req.RunAt, time.Now()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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

	if req.RunAt != "" {
		if err := rt.appSchedules.SetAppScheduleRunAt(r.Context(), name, req.Branch, timezone, runAt, enabled); err != nil {
			rt.internalError(w, "api: set one-time app schedule failed", err, slog.String("name", name))
			return
		}
		resource := appScheduleResource{ServiceName: name, Branch: req.Branch, Timezone: timezone, Enabled: enabled, RunAt: &runAt}
		resource.NextRunAt = &runAt
		writeJSON(w, http.StatusOK, resource)
		return
	}

	if err := rt.appSchedules.SetAppSchedule(r.Context(), name, req.Cron, req.Branch, timezone, enabled); err != nil {
		rt.internalError(w, "api: set app schedule failed", err, slog.String("name", name))
		return
	}

	resource := appScheduleResource{ServiceName: name, Cron: req.Cron, Branch: req.Branch, Timezone: timezone, Enabled: enabled}
	// Arm next_fire_at now, not on the scheduler's next tick, so a just-saved schedule shows its next run at once.
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

// parseAppScheduleRunAt parses an RFC 3339 run_at and checks it lies between
// the configured minimum lead and maximum horizon from now.
func parseAppScheduleRunAt(raw string, now time.Time) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, errors.New("invalid run_at: must be an RFC 3339 timestamp such as 2026-12-01T03:00:00Z")
	}
	minLead := envDuration(envAppScheduleRunAtMinLead, defaultAppScheduleRunAtMinLead)
	maxHorizon := envDuration(envAppScheduleRunAtMaxHorizon, defaultAppScheduleRunAtMaxHorizon)
	if at.Before(now.Add(minLead)) {
		return time.Time{}, fmt.Errorf("run_at must be at least %s in the future", minLead)
	}
	if at.After(now.Add(maxHorizon)) {
		return time.Time{}, fmt.Errorf("run_at must be within %s from now", maxHorizon)
	}
	return at.UTC(), nil
}

type appScheduleHistoryResource struct {
	ID           string    `json:"id"`
	ScheduledFor time.Time `json:"scheduled_for"`
	FiredAt      time.Time `json:"fired_at"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
}

// handleListAppScheduleHistory returns recent schedule evaluations, newest first, including skipped freeze windows.
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
		rt.internalError(w, "api: list app schedule history failed", err, slog.String("name", name))
		return
	}

	out := make([]appScheduleHistoryResource, 0, len(entries))
	for _, e := range entries {
		out = append(out, appScheduleHistoryResource{ID: e.ID, ScheduledFor: e.ScheduledFor, FiredAt: e.FiredAt, Status: e.Status, Reason: e.Reason})
	}
	writeJSON(w, http.StatusOK, out)
}
