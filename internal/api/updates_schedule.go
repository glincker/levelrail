package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgradesched"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	maxPolicyChannels   = 32
	auditActionSchedule = "upgrade.schedule"
	auditActionPolicy   = "upgrade.policy"
	atLocalLayout       = "2006-01-02T15:04"
)

type upgradeScheduleResource struct {
	ID            string   `json:"id"`
	TargetVersion string   `json:"target_version"`
	Source        string   `json:"source"`
	Mode          string   `json:"mode"`
	State         string   `json:"state"`
	RunAt         string   `json:"run_at"`
	RetryUntil    string   `json:"retry_until"`
	Timezone      string   `json:"timezone"`
	Acked         []string `json:"acked"`
	MajorAcked    bool     `json:"major_acked"`
	CreatedBy     string   `json:"created_by"`
	CreatedAt     string   `json:"created_at"`
	CheckCount    int      `json:"check_count"`
	LastCheckAt   string   `json:"last_check_at,omitempty"`
	LastReason    string   `json:"last_reason,omitempty"`
	FiredAt       string   `json:"fired_at,omitempty"`
	FinishedAt    string   `json:"finished_at,omitempty"`
}

func toUpgradeScheduleResource(s store.UpgradeSchedule) upgradeScheduleResource {
	out := upgradeScheduleResource{
		ID: s.ID, TargetVersion: s.TargetVersion, Source: s.Source, Mode: s.Mode, State: s.State, RunAt: s.RunAt,
		RetryUntil: s.RetryUntil, Timezone: s.Timezone, Acked: []string{}, MajorAcked: s.MajorAcked, CreatedBy: s.CreatedBy,
		CreatedAt: s.CreatedAt, CheckCount: s.CheckCount, LastCheckAt: s.LastCheckAt, LastReason: s.LastReason,
		FiredAt: s.FiredAt, FinishedAt: s.FinishedAt,
	}
	_ = json.Unmarshal([]byte(s.AckedJSON), &out.Acked)
	return out
}

type upgradeWindowResource struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Source string `json:"source"`
	Active bool   `json:"active"`
}

type upgradeScheduleStatus struct {
	Available      bool                      `json:"available"`
	ServerTimezone string                    `json:"server_timezone"`
	Now            string                    `json:"now"`
	Active         *upgradeScheduleResource  `json:"active"`
	Recent         []upgradeScheduleResource `json:"recent"`
	NextWindow     *upgradeWindowResource    `json:"next_window"`
}

// serverTimezone names the zone this server's wall clock is in, so the
// dashboard can show "03:00 (Europe/Berlin)" rather than an ambiguous time.
func serverTimezone() string {
	if name := time.Now().Location().String(); name != "Local" && name != "" {
		return name
	}
	if tz := strings.TrimSpace(os.Getenv("TZ")); tz != "" {
		if _, err := time.LoadLocation(tz); err == nil {
			return tz
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(target, "zoneinfo/"); i >= 0 {
			return target[i+len("zoneinfo/"):]
		}
	}
	return "UTC"
}

func (rt *Router) upgradeSchedulerOr501(w http.ResponseWriter) *upgradesched.Scheduler {
	s := rt.UpgradeScheduler()
	if s == nil {
		writeError(w, http.StatusNotImplemented, "upgrade scheduling is not configured on this control plane")
	}
	return s
}

// handleGetUpgradeSchedule handles GET /api/v1/updates/schedule.
func (rt *Router) handleGetUpgradeSchedule(w http.ResponseWriter, r *http.Request) {
	out := upgradeScheduleStatus{ServerTimezone: serverTimezone(), Now: time.Now().UTC().Format(time.RFC3339), Recent: []upgradeScheduleResource{}}
	s := rt.UpgradeScheduler()
	if s == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Available = true
	list, err := rt.upgradeSchedule.store.ListUpgradeSchedules(r.Context(), 6)
	if err != nil {
		rt.internalError(w, "api: list upgrade schedules failed", err)
		return
	}
	for _, sc := range list {
		res := toUpgradeScheduleResource(sc)
		if sc.IsActive() && out.Active == nil {
			out.Active = &res
			continue
		}
		out.Recent = append(out.Recent, res)
	}
	out.NextWindow = rt.nextUpgradeWindow(r.Context(), s)
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) nextUpgradeWindow(ctx context.Context, s *upgradesched.Scheduler) *upgradeWindowResource {
	now := time.Now().UTC()
	span, ok, err := s.NextWindow(ctx, now)
	if err != nil || !ok {
		return nil
	}
	source := "alert_maintenance_window"
	if row, err := rt.upgradeSchedule.store.GetUpgradePolicy(ctx); err == nil && row.WindowCron != "" {
		source = "upgrade_policy"
	}
	return &upgradeWindowResource{
		Start: span.Start.UTC().Format(time.RFC3339), End: span.End.UTC().Format(time.RFC3339),
		Source: source, Active: !span.Start.After(now),
	}
}

type upgradeScheduleRequest struct {
	Target           string   `json:"target"`
	At               string   `json:"at"`
	AtLocal          string   `json:"at_local"`
	Timezone         string   `json:"timezone"`
	Window           bool     `json:"window"`
	Ack              []string `json:"ack"`
	AcknowledgeMajor bool     `json:"acknowledge_major"`
	Replace          bool     `json:"replace"`
}

// handleCreateUpgradeSchedule handles POST /api/v1/updates/schedule. Root only.
func (rt *Router) handleCreateUpgradeSchedule(w http.ResponseWriter, r *http.Request) {
	s := rt.upgradeSchedulerOr501(w)
	if s == nil {
		return
	}
	var req upgradeScheduleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Ack) > maxAckIDs {
		writeError(w, http.StatusBadRequest, "too many acknowledgements")
		return
	}
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return
	}
	in := upgradesched.Request{
		Target: strings.TrimSpace(req.Target), Window: req.Window, Ack: req.Ack, MajorAck: req.AcknowledgeMajor,
		Replace: req.Replace, Actor: sanitizeInitiator(firstNonEmpty(actorName, actorID)),
	}
	tz := serverTimezone()
	if t := strings.TrimSpace(req.Timezone); t != "" {
		tz = t
	}
	in.Timezone = tz
	if !req.Window {
		at, err := parseScheduleTime(req, tz)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		in.At = at
	}
	sc, err := s.Schedule(r.Context(), in)
	if err != nil {
		rt.writeUpgradeScheduleError(w, "schedule upgrade", err)
		return
	}
	rt.auditUpgradeEvent(r.Context(), r, auditActionSchedule, fmt.Sprintf("scheduled %s for %s (%s)", sc.TargetVersion, sc.RunAt, sc.ID))
	writeJSON(w, http.StatusCreated, toUpgradeScheduleResource(sc))
}

func parseScheduleTime(req upgradeScheduleRequest, tz string) (time.Time, error) {
	switch {
	case strings.TrimSpace(req.At) != "":
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(req.At))
		if err != nil {
			return time.Time{}, errors.New("at must be an RFC 3339 time such as 2026-10-12T03:00:00Z")
		}
		return t, nil
	case strings.TrimSpace(req.AtLocal) != "":
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return time.Time{}, fmt.Errorf("unknown timezone %q", tz)
		}
		t, err := time.ParseInLocation(atLocalLayout, strings.TrimSpace(req.AtLocal), loc)
		if err != nil {
			return time.Time{}, errors.New("at_local must look like 2026-10-12T03:00")
		}
		return t, nil
	}
	return time.Time{}, errors.New("give at, at_local with a timezone, or window")
}

func (rt *Router) writeUpgradeScheduleError(w http.ResponseWriter, what string, err error) {
	var ack *upgradesched.AckError
	switch {
	case errors.As(err, &ack):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "breaking changes need acknowledgement", "missing_ack": ack.IDs})
	case errors.Is(err, upgradesched.ErrNeedsMajorAck):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "a major version jump needs acknowledgement", "needs_major_ack": true})
	case errors.Is(err, upgradesched.ErrAlreadyScheduled):
		writeError(w, http.StatusConflict, "an upgrade is already scheduled; cancel it or replace it")
	case errors.Is(err, upgradesched.ErrRunning):
		writeError(w, http.StatusConflict, "the upgrade is already running and cannot be changed")
	case errors.Is(err, upgradesched.ErrNoWindow):
		writeError(w, http.StatusConflict, "no maintenance window is configured: set one under automatic upgrades, or pick a time")
	case errors.Is(err, upgradesched.ErrEdgeTarget):
		writeError(w, http.StatusConflict, errEdgeNotApplicable)
	case errors.Is(err, upgradesched.ErrReleaseNotFound):
		writeError(w, http.StatusNotFound, "that release was not found on the configured channel")
	case errors.Is(err, upgradesched.ErrNothingScheduled):
		writeError(w, http.StatusNotFound, "no upgrade is scheduled")
	case errors.Is(err, upgradesched.ErrInvalid):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), upgradesched.ErrInvalid.Error()+": "))
	default:
		rt.internalError(w, "api: "+what+" failed", err)
	}
}

// handleCancelUpgradeSchedule handles DELETE /api/v1/updates/schedule. Root only.
func (rt *Router) handleCancelUpgradeSchedule(w http.ResponseWriter, r *http.Request) {
	s := rt.upgradeSchedulerOr501(w)
	if s == nil {
		return
	}
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return
	}
	sc, err := s.Cancel(r.Context(), sanitizeInitiator(firstNonEmpty(actorName, actorID)))
	if err != nil {
		rt.writeUpgradeScheduleError(w, "cancel upgrade schedule", err)
		return
	}
	rt.auditUpgradeEvent(r.Context(), r, auditActionSchedule, fmt.Sprintf("cancelled %s (%s)", sc.TargetVersion, sc.ID))
	writeJSON(w, http.StatusOK, toUpgradeScheduleResource(sc))
}

type upgradePolicyResource struct {
	Mode                  string                 `json:"mode"`
	Effective             string                 `json:"effective"`
	WindowCron            string                 `json:"window_cron"`
	WindowDurationMinutes int                    `json:"window_duration_minutes"`
	Timezone              string                 `json:"timezone"`
	MinReleaseAgeDays     int                    `json:"min_release_age_days"`
	NotifyChannelIDs      []string               `json:"notify_channel_ids"`
	Paused                bool                   `json:"paused"`
	UpdatedBy             string                 `json:"updated_by,omitempty"`
	UpdatedAt             string                 `json:"updated_at,omitempty"`
	Decision              string                 `json:"decision,omitempty"`
	DecisionTag           string                 `json:"decision_tag,omitempty"`
	DecisionAt            string                 `json:"decision_at,omitempty"`
	ServerTimezone        string                 `json:"server_timezone"`
	NextWindow            *upgradeWindowResource `json:"next_window"`
}

func (rt *Router) upgradePolicyResource(row store.UpgradePolicy) upgradePolicyResource {
	out := upgradePolicyResource{
		Mode: row.Mode, Effective: row.Mode, WindowCron: row.WindowCron, WindowDurationMinutes: row.WindowDurationSeconds / 60,
		Timezone: row.WindowTimezone, MinReleaseAgeDays: row.MinReleaseAgeDays, NotifyChannelIDs: []string{}, Paused: row.Paused,
		UpdatedBy: row.UpdatedBy, UpdatedAt: row.UpdatedAt, Decision: row.Decision, DecisionTag: row.DecisionTag,
		DecisionAt: row.DecisionAt, ServerTimezone: serverTimezone(),
	}
	if row.Paused && row.Mode != upgradesched.ModeOff {
		out.Effective = "paused"
	}
	_ = json.Unmarshal([]byte(row.NotifyChannelIDs), &out.NotifyChannelIDs)
	if row.WindowCron != "" {
		w := upgradesched.Window{Cron: row.WindowCron, Duration: time.Duration(row.WindowDurationSeconds) * time.Second, Timezone: row.WindowTimezone}
		now := time.Now().UTC()
		if span, ok, err := w.Next(now); err == nil && ok {
			out.NextWindow = &upgradeWindowResource{
				Start: span.Start.UTC().Format(time.RFC3339), End: span.End.UTC().Format(time.RFC3339),
				Source: "upgrade_policy", Active: !span.Start.After(now),
			}
		}
	}
	return out
}

// handleGetUpgradePolicy handles GET /api/v1/updates/auto. Root only.
func (rt *Router) handleGetUpgradePolicy(w http.ResponseWriter, r *http.Request) {
	if rt.upgradeSchedule == nil {
		writeError(w, http.StatusNotImplemented, "upgrade scheduling is not configured on this control plane")
		return
	}
	row, err := rt.upgradeSchedule.store.GetUpgradePolicy(r.Context())
	if err != nil {
		rt.internalError(w, "api: get upgrade policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.upgradePolicyResource(row))
}

type upgradePolicyRequest struct {
	Mode                  string   `json:"mode"`
	WindowCron            string   `json:"window_cron"`
	WindowDurationMinutes int      `json:"window_duration_minutes"`
	Timezone              string   `json:"timezone"`
	MinReleaseAgeDays     int      `json:"min_release_age_days"`
	NotifyChannelIDs      []string `json:"notify_channel_ids"`
	Paused                bool     `json:"paused"`
}

// handleUpdateUpgradePolicy handles PUT /api/v1/updates/auto. Root only.
func (rt *Router) handleUpdateUpgradePolicy(w http.ResponseWriter, r *http.Request) {
	if rt.upgradeSchedule == nil {
		writeError(w, http.StatusNotImplemented, "upgrade scheduling is not configured on this control plane")
		return
	}
	var req upgradePolicyRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.NotifyChannelIDs) > maxPolicyChannels {
		writeError(w, http.StatusBadRequest, "too many notification channels")
		return
	}
	if req.Timezone == "" {
		req.Timezone = serverTimezone()
	}
	if req.WindowDurationMinutes == 0 {
		req.WindowDurationMinutes = 120
	}
	pol := upgradesched.Policy{
		Mode: req.Mode, MinReleaseAgeDays: req.MinReleaseAgeDays, Paused: req.Paused,
		Window: upgradesched.Window{Cron: strings.TrimSpace(req.WindowCron), Duration: time.Duration(req.WindowDurationMinutes) * time.Minute, Timezone: req.Timezone},
	}
	if err := pol.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if pol.Active() && rt.currentUpdateChannel(r.Context()) == upgrade.ChannelEdge {
		writeError(w, http.StatusConflict, "automatic upgrades cannot run on the edge channel: edge builds are container images. Switch the channel to beta or stable first")
		return
	}
	if rt.notificationChannels != nil {
		known, err := rt.notificationChannels.ListNotificationChannels(r.Context())
		if err != nil {
			rt.internalError(w, "api: list notification channels failed", err)
			return
		}
		have := map[string]bool{}
		for _, c := range known {
			have[c.ID] = true
		}
		for _, id := range req.NotifyChannelIDs {
			if !have[id] {
				writeError(w, http.StatusBadRequest, "unknown notification channel "+id)
				return
			}
		}
	}
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return
	}
	ids := req.NotifyChannelIDs
	if ids == nil {
		ids = []string{}
	}
	rawIDs, _ := json.Marshal(ids)
	before, err := rt.upgradeSchedule.store.GetUpgradePolicy(r.Context())
	if err != nil {
		rt.internalError(w, "api: get upgrade policy failed", err)
		return
	}
	row := store.UpgradePolicy{
		Mode: req.Mode, WindowCron: pol.Window.Cron, WindowDurationSeconds: int(pol.Window.Duration.Seconds()),
		WindowTimezone: req.Timezone, MinReleaseAgeDays: req.MinReleaseAgeDays, NotifyChannelIDs: string(rawIDs),
		Paused: req.Paused, UpdatedBy: sanitizeInitiator(firstNonEmpty(actorName, actorID)),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := rt.upgradeSchedule.store.UpdateUpgradePolicy(r.Context(), row); err != nil {
		rt.internalError(w, "api: update upgrade policy failed", err)
		return
	}
	rt.auditUpgradeEvent(r.Context(), r, auditActionPolicy, fmt.Sprintf("mode %s -> %s, paused=%t, window %q %s, min age %dd",
		before.Mode, row.Mode, row.Paused, row.WindowCron, row.WindowTimezone, row.MinReleaseAgeDays))
	rt.logger.Info("api: upgrade policy updated", slog.String("mode", row.Mode), slog.Bool("paused", row.Paused), slog.String("by", row.UpdatedBy))
	updated, err := rt.upgradeSchedule.store.GetUpgradePolicy(r.Context())
	if err != nil {
		rt.internalError(w, "api: get upgrade policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.upgradePolicyResource(updated))
}

type upgradeBackupNowResponse struct {
	Backup    map[string]any          `json:"backup"`
	Preflight updatePreflightResource `json:"preflight"`
}

// handleUpgradeBackupNow handles POST /api/v1/updates/backup-now. Root only.
// It takes a control plane backup and returns the preflight evaluated after
// it, so a stale-backup blocker clears in one round trip.
func (rt *Router) handleUpgradeBackupNow(w http.ResponseWriter, r *http.Request) {
	m, ok := rt.cpBackupsOrNotImplemented(w)
	if !ok {
		return
	}
	info, err := m.Create(r.Context())
	if err != nil {
		rt.writeCPBackupError(w, "create control plane backup", err)
		return
	}
	rt.auditUpgradeEvent(r.Context(), r, "upgrade.backup", "control plane backup "+info.Name+" taken for an upgrade")
	release, known := rt.latestReleaseForCurrentChannel(r.Context())
	writeJSON(w, http.StatusCreated, upgradeBackupNowResponse{
		Backup:    map[string]any{"name": info.Name, "size_bytes": info.SizeBytes, "created_at": info.CreatedAt},
		Preflight: rt.evaluatePreflight(r.Context(), release, known && release != nil),
	})
}

// auditUpgradeEvent writes a descriptive audit row next to the generic one
// the request middleware records.
func (rt *Router) auditUpgradeEvent(ctx context.Context, r *http.Request, action, detail string) {
	actorType, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: upgrade audit id failed", slog.String("error", err.Error()))
		return
	}
	err = rt.auditLog.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName, Ability: AbilityRoot,
		Method: r.Method, Path: action + ": " + detail, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: action,
	})
	if err != nil {
		rt.logger.Warn("api: upgrade audit failed", slog.String("error", err.Error()))
	}
}

// auditSystemUpgradeEvent records a scheduler-initiated upgrade event.
func (rt *Router) auditSystemUpgradeEvent(ctx context.Context, detail string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	err = rt.auditLog.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: "system", ActorName: "upgrade-scheduler", Ability: AbilityRoot, Method: http.MethodPost,
		Path: "upgrade.scheduler: " + detail, StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: "system", Action: "upgrade.scheduler",
	})
	if err != nil {
		rt.logger.Warn("api: upgrade scheduler audit failed", slog.String("error", err.Error()))
	}
}
