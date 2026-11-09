package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// Runtime settings for scheduled restarts; all apply on the next sweep or request.
const (
	KeyRestartScheduleMinInterval = "deploy.restart_schedule_min_interval"
	KeyRestartScheduleMaxPerSweep = "deploy.restart_schedule_max_per_sweep"
	KeyRestartScheduleBackoff     = "deploy.restart_schedule_failure_backoff"

	envRestartScheduleTick = "APP_RESTART_SCHEDULE_TICK"
)

type durationBounds struct{ def, lo, hi time.Duration }

var (
	restartMinIntervalBounds = durationBounds{time.Hour, 5 * time.Minute, 7 * 24 * time.Hour}
	restartBackoffBounds     = durationBounds{15 * time.Minute, time.Minute, 24 * time.Hour}
	restartTickBounds        = durationBounds{time.Minute, 10 * time.Second, 10 * time.Minute}
	restartMaxPerSweepBounds = expirySettingBounds{5, 1, 100}
)

const restartBackoffCeiling = 24 * time.Hour

func durationCheck(b durationBounds) func(string) error {
	return func(canonical string) error {
		if d, err := time.ParseDuration(canonical); err != nil || d < b.lo || d > b.hi {
			return fmt.Errorf("value must be between %s and %s", b.lo, b.hi)
		}
		return nil
	}
}

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyRestartScheduleMinInterval, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindDuration,
		Default: restartMinIntervalBounds.def.String(), Env: "APP_RESTART_SCHEDULE_MIN_INTERVAL", Apply: runtimesettings.ApplyLive,
		CheckValue:  durationCheck(restartMinIntervalBounds),
		Title:       "Scheduled restart minimum interval",
		Description: "Shortest gap allowed between two scheduled restarts of one app, so a schedule cannot restart-loop an app. Between 5m and 168h.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyRestartScheduleMaxPerSweep, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: strconv.Itoa(restartMaxPerSweepBounds.def), Env: "APP_RESTART_SCHEDULE_MAX_PER_SWEEP", Apply: runtimesettings.ApplyLive,
		CheckValue: func(canonical string) error {
			b := restartMaxPerSweepBounds
			if n, err := strconv.Atoi(canonical); err != nil || n < b.lo || n > b.hi {
				return fmt.Errorf("value must be between %d and %d", b.lo, b.hi)
			}
			return nil
		},
		Title:       "Scheduled restarts per sweep",
		Description: "Most apps one control plane restarts in one scheduled restart sweep. The rest wait for the next sweep.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyRestartScheduleBackoff, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindDuration,
		Default: restartBackoffBounds.def.String(), Env: "APP_RESTART_SCHEDULE_FAILURE_BACKOFF", Apply: runtimesettings.ApplyLive,
		CheckValue:  durationCheck(restartBackoffBounds),
		Title:       "Scheduled restart failure backoff",
		Description: "Wait before retrying a scheduled restart that failed, doubled per consecutive failure up to 24h, and never later than the next regular run. Between 1m and 24h.",
	})
}

func (rt *Router) restartDuration(ctx context.Context, key string, b durationBounds) time.Duration {
	d := b.def
	if rt.runtimeSettings != nil {
		if v := rt.runtimeSettings.Duration(ctx, key); v > 0 {
			d = v
		}
	}
	return min(max(d, b.lo), b.hi)
}

func (rt *Router) restartMaxPerSweep(ctx context.Context) int {
	b := restartMaxPerSweepBounds
	n := b.def
	if rt.runtimeSettings != nil {
		n = rt.runtimeSettings.Int(ctx, KeyRestartScheduleMaxPerSweep)
	}
	return min(max(n, b.lo), b.hi)
}

// RestartScheduleTick is how often the sweep looks for due restarts, from env and clamped.
func RestartScheduleTick() time.Duration {
	d, err := time.ParseDuration(os.Getenv(envRestartScheduleTick))
	if err != nil {
		d = restartTickBounds.def
	}
	return min(max(d, restartTickBounds.lo), restartTickBounds.hi)
}

type appRestartScheduleStore interface {
	GetAppRestartSchedule(ctx context.Context, app string) (*store.AppRestartSchedule, error)
	SetAppRestartSchedule(ctx context.Context, s store.AppRestartSchedule) error
	ClearAppRestartSchedule(ctx context.Context, app string) (bool, error)
	ListDueAppRestartSchedules(ctx context.Context, now string, limit int) ([]store.AppRestartSchedule, error)
	RecordAppRestartRun(ctx context.Context, prev store.AppRestartSchedule, ranAt, status, reason string, failures int, next string) (bool, error)
	RemoveAppRestartScheduleRow(ctx context.Context, s store.AppRestartSchedule) error
}

// restartNext returns the next fire after a time for a validated schedule.
type restartNext func(after time.Time) time.Time

const restartCronSample = 64

// parseRestartSchedule accepts a 5 field cron or "@every <duration>" and
// refuses any schedule whose shortest gap is under the floor.
func parseRestartSchedule(expr string, loc *time.Location, now time.Time, floor time.Duration) (restartNext, error) {
	expr = strings.TrimSpace(expr)
	if rest, ok := strings.CutPrefix(expr, "@every "); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil || d <= 0 {
			return nil, errors.New("schedule: @every needs a positive duration, for example @every 24h")
		}
		if d < floor {
			return nil, fmt.Errorf("schedule: restarts must be at least %s apart", floor)
		}
		return func(after time.Time) time.Time { return after.Add(d) }, nil
	}
	if err := cronexpr.Validate(expr, loc, now); err != nil {
		return nil, fmt.Errorf("schedule: %w", err)
	}
	sched, err := cronexpr.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("schedule: %w", err)
	}
	next := func(after time.Time) time.Time { return cronexpr.NextInLocation(sched, after, loc) }
	prev := next(now)
	for i := 0; i < restartCronSample && !prev.IsZero(); i++ {
		t := next(prev)
		if t.IsZero() {
			break
		}
		if t.Sub(prev) < floor {
			return nil, fmt.Errorf("schedule: restarts must be at least %s apart, this schedule fires %s apart", floor, t.Sub(prev))
		}
		prev = t
	}
	return next, nil
}

type appRestartScheduleResource struct {
	App         string                    `json:"app"`
	Schedule    *store.AppRestartSchedule `json:"schedule,omitempty"`
	MinInterval string                    `json:"min_interval"`
}

type setAppRestartScheduleRequest struct {
	Schedule string `json:"schedule"`
	Timezone string `json:"timezone"`
}

func (rt *Router) toAppRestartScheduleResource(ctx context.Context, app string, s *store.AppRestartSchedule) appRestartScheduleResource {
	return appRestartScheduleResource{App: app, Schedule: s, MinInterval: rt.restartDuration(ctx, KeyRestartScheduleMinInterval, restartMinIntervalBounds).String()}
}

// loadRestartScheduleApp writes the 404 itself, so callers only see apps that exist.
func (rt *Router) loadRestartScheduleApp(w http.ResponseWriter, r *http.Request) (*store.DesiredService, appRestartScheduleStore, bool) {
	name := r.PathValue("name")
	st, ok := rt.apps.(appRestartScheduleStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "scheduled restarts are not available")
		return nil, nil, false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		rt.writeAppNotFound(r.Context(), w, name)
		return nil, nil, false
	} else if err != nil {
		rt.internalError(w, "api: restart schedule: load app", err, slog.String("name", name))
		return nil, nil, false
	}
	return svc, st, true
}

// handleGetAppRestartSchedule handles GET /api/v1/apps/{name}/restart-schedule.
func (rt *Router) handleGetAppRestartSchedule(w http.ResponseWriter, r *http.Request) {
	svc, st, ok := rt.loadRestartScheduleApp(w, r)
	if !ok {
		return
	}
	s, err := st.GetAppRestartSchedule(r.Context(), svc.Name)
	if err != nil {
		rt.internalError(w, "api: get restart schedule", err, slog.String("name", svc.Name))
		return
	}
	if s != nil && s.Incarnation != svc.Incarnation {
		s = nil // a row from an earlier life of the name is not this app's
	}
	writeJSON(w, http.StatusOK, rt.toAppRestartScheduleResource(r.Context(), svc.Name, s))
}

// handleSetAppRestartSchedule handles PUT /api/v1/apps/{name}/restart-schedule. It needs
// deploy, like a manual restart, since a restart also makes staged env edits live.
func (rt *Router) handleSetAppRestartSchedule(w http.ResponseWriter, r *http.Request) {
	svc, st, ok := rt.loadRestartScheduleApp(w, r)
	if !ok {
		return
	}
	name := svc.Name
	var req setAppRestartScheduleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tz := strings.TrimSpace(req.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		writeError(w, http.StatusBadRequest, "timezone must be an IANA zone, for example Europe/Berlin")
		return
	}
	now := time.Now().UTC()
	floor := rt.restartDuration(r.Context(), KeyRestartScheduleMinInterval, restartMinIntervalBounds)
	next, err := parseRestartSchedule(req.Schedule, loc, now, floor)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	unlock := lockAppName(name)
	defer unlock()
	if cur, err := rt.apps.GetDesiredService(r.Context(), name); err != nil || cur.Incarnation != svc.Incarnation {
		writeError(w, http.StatusConflict, "the app changed while the schedule was being set; try again")
		return
	}
	incarnation := svc.Incarnation
	if is, ok := rt.apps.(serviceIncarnationStore); ok && incarnation == "" {
		var updated bool
		if incarnation, updated, err = is.ClaimServiceIncarnation(r.Context(), name); err != nil {
			rt.internalError(w, "api: set restart schedule: incarnation", err, slog.String("name", name))
			return
		}
		if !updated {
			writeError(w, http.StatusConflict, "the app changed while the schedule was being set; try again")
			return
		}
	}
	if incarnation == "" {
		writeError(w, http.StatusConflict, "this app has no stable identity yet; redeploy it, then set the schedule")
		return
	}
	nextRun := next(now)
	// Re-saving must not pull the next run inside the floor of the last one.
	if prev, perr := st.GetAppRestartSchedule(r.Context(), name); perr != nil {
		rt.internalError(w, "api: set restart schedule: load current", perr, slog.String("name", name))
		return
	} else if prev != nil && prev.Incarnation == incarnation && prev.LastRunAt != "" {
		if last, lerr := time.Parse(time.RFC3339, prev.LastRunAt); lerr == nil && nextRun.Before(last.Add(floor)) {
			nextRun = last.Add(floor)
		}
	}
	s := store.AppRestartSchedule{AppName: name, Schedule: strings.TrimSpace(req.Schedule), Timezone: tz, Incarnation: incarnation,
		SetBy: rt.actorLabel(r), SetAt: now.Format(time.RFC3339), NextRunAt: nextRun.UTC().Format(time.RFC3339)}
	if err := st.SetAppRestartSchedule(r.Context(), s); err != nil {
		rt.internalError(w, "api: set restart schedule", err, slog.String("name", name))
		return
	}
	rt.auditRestartScheduleChange(r, "app.restart_schedule.set", name, map[string]any{"app": name, "schedule": s.Schedule, "timezone": tz})
	saved, err := st.GetAppRestartSchedule(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set restart schedule: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toAppRestartScheduleResource(r.Context(), name, saved))
}

// handleClearAppRestartSchedule handles DELETE /api/v1/apps/{name}/restart-schedule (deploy).
func (rt *Router) handleClearAppRestartSchedule(w http.ResponseWriter, r *http.Request) {
	unlock := lockAppName(r.PathValue("name"))
	defer unlock()
	svc, st, ok := rt.loadRestartScheduleApp(w, r)
	if !ok {
		return
	}
	existed, err := st.ClearAppRestartSchedule(r.Context(), svc.Name)
	if err != nil {
		rt.internalError(w, "api: clear restart schedule", err, slog.String("name", svc.Name))
		return
	}
	if existed {
		rt.auditRestartScheduleChange(r, "app.restart_schedule.clear", svc.Name, map[string]any{"app": svc.Name})
	}
	writeJSON(w, http.StatusOK, rt.toAppRestartScheduleResource(r.Context(), svc.Name, nil))
}

func (rt *Router) auditRestartScheduleChange(r *http.Request, action, app string, detail map[string]any) {
	a := rt.accessActor(r)
	b, _ := json.Marshal(detail)
	rt.saveAppExpiryAudit(r.Context(), store.AuditEntry{
		ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: r.Method, Path: "/api/v1/apps/" + app + "/restart-schedule", StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: action, Detail: string(b),
	})
}

// recordRestartScheduleAudit writes a system row naming only the app and the outcome.
func (rt *Router) recordRestartScheduleAudit(ctx context.Context, action, app, reason string) {
	rt.saveAppExpiryAudit(ctx, store.AuditEntry{
		ActorType: auditActorSystem, ActorID: "restart-schedule", ActorName: "scheduled restart",
		Ability: AbilityRoot, Method: http.MethodPost, Path: "/api/v1/apps/" + app + "/restart", StatusCode: http.StatusOK,
		ClientKind: "system", Action: action, Detail: appExpiryAuditDetail(app, reason),
	})
}

// RunAppRestartScheduler sweeps due scheduled restarts on interval until ctx is done.
func (rt *Router) RunAppRestartScheduler(ctx context.Context, interval time.Duration) error {
	return footprint.Every(ctx, "restart schedule", interval, rt.logger, func(ctx context.Context) error {
		_, err := rt.SweepAppRestartSchedules(ctx, time.Now())
		return err
	})
}

const restartDueListLimit = 200

// SweepAppRestartSchedules restarts due apps through the manual restart path,
// at most the per-sweep cap, skipping any app it must not touch now.
func (rt *Router) SweepAppRestartSchedules(ctx context.Context, now time.Time) (int, error) {
	return rt.sweepAppRestartSchedules(ctx, now, restartDueListLimit)
}

func (rt *Router) sweepAppRestartSchedules(ctx context.Context, now time.Time, listLimit int) (restarted int, err error) {
	st, ok := rt.apps.(appRestartScheduleStore)
	if !ok {
		return 0, nil
	}
	now = now.UTC()
	due, err := st.ListDueAppRestartSchedules(ctx, now.Format(time.RFC3339), listLimit)
	if err != nil {
		return 0, fmt.Errorf("api: sweep restart schedules: %w", err)
	}
	limit := rt.restartMaxPerSweep(ctx)
	var errs []error
	for _, s := range due {
		if restarted >= limit {
			break
		}
		did, rerr := rt.runScheduledRestart(ctx, st, s, now)
		if rerr != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", s.AppName, rerr))
		}
		if did {
			restarted++
		}
	}
	return restarted, errors.Join(errs...)
}

// restartSkipReason says why the sweep must not restart svc now; a lookup
// error leaves the row unchanged so the next sweep looks again.
func (rt *Router) restartSkipReason(ctx context.Context, svc store.DesiredService, now time.Time) (string, error) {
	if svc.Suspended {
		return "app is stopped", nil
	}
	if svc.EnvironmentID != "" {
		if rt.environments == nil {
			return "environment protection unknown", nil
		}
		env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID)
		if err != nil {
			return "", err
		}
		if env.Protected {
			return "app is in a protected environment", nil
		}
	}
	if rt.deploySafety != nil {
		status, err := deploy.CheckFreeze(ctx, rt.deploySafety, svc.Name, now)
		if err != nil {
			return "", err
		}
		if status.Frozen {
			return "a deploy freeze is active", nil
		}
	}
	if busy, err := rt.appDeployRunning(ctx, svc.Name); err != nil {
		return "", err
	} else if busy {
		return "a deploy is running", nil
	}
	if rt.offlineNodeFor(ctx, svc) != nil {
		return "the app's node is offline", nil
	}
	// checkDisruption fails open for an operator; the unattended sweep must not.
	if _, err := rt.rolloutPolicy(ctx, svc.Name); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", nil)
	if err != nil {
		return "", err
	}
	if derr := rt.checkDisruption(req, svc, disruptionRestart); derr != nil {
		return "disruption budget: " + derr.Error(), nil
	}
	return "", nil
}

func restartBackoff(base time.Duration, failures int) time.Duration {
	d := base
	for i := 1; i < failures && d < restartBackoffCeiling; i++ {
		d *= 2
	}
	return min(d, restartBackoffCeiling)
}

// runScheduledRestart claims the row by moving next_run_at first, then
// restarts, so a crash or a second control plane never fires it twice.
func (rt *Router) runScheduledRestart(ctx context.Context, st appRestartScheduleStore, s store.AppRestartSchedule, now time.Time) (bool, error) {
	unlock := lockAppName(s.AppName)
	defer unlock()
	svc, err := rt.apps.GetDesiredService(ctx, s.AppName)
	if errors.Is(err, store.ErrServiceNotFound) || (err == nil && (s.Incarnation == "" || svc.Incarnation != s.Incarnation)) {
		return false, st.RemoveAppRestartScheduleRow(ctx, s) // another life of the name never inherits this row
	}
	if err != nil {
		return false, rt.backOffRestartRow(ctx, st, s, now, time.Time{}, err)
	}
	stamp := now.Format(time.RFC3339)
	floor := rt.restartDuration(ctx, KeyRestartScheduleMinInterval, restartMinIntervalBounds)
	loc, lerr := time.LoadLocation(s.Timezone)
	if lerr != nil {
		loc = time.UTC
	}
	next, perr := parseRestartSchedule(s.Schedule, loc, now, floor)
	if perr != nil {
		// A raised floor or a vanished zone leaves the row paused at the floor, never firing faster.
		ok, rerr := st.RecordAppRestartRun(ctx, s, stamp, store.AppRestartStatusSkipped, perr.Error(), s.Failures, now.Add(floor).Format(time.RFC3339))
		if ok && rerr == nil {
			rt.recordRestartScheduleAudit(ctx, "app.restart_schedule.skipped", s.AppName, "schedule no longer valid")
		}
		return false, rerr
	}
	regular := next(now)
	if regular.IsZero() {
		regular = now.Add(floor)
	}
	regularStamp := regular.UTC().Format(time.RFC3339)
	reason, err := rt.restartSkipReason(ctx, *svc, now)
	if err != nil {
		return false, rt.backOffRestartRow(ctx, st, s, now, regular, err)
	}
	if reason != "" {
		if ok, rerr := st.RecordAppRestartRun(ctx, s, stamp, store.AppRestartStatusSkipped, reason, s.Failures, regularStamp); rerr != nil || !ok {
			return false, rerr
		}
		rt.recordRestartScheduleAudit(ctx, "app.restart_schedule.skipped", s.AppName, reason)
		return false, nil
	}
	ok, err := st.RecordAppRestartRun(ctx, s, stamp, store.AppRestartStatusStarted, "", s.Failures, regularStamp)
	if err != nil || !ok {
		return false, err
	}
	claimed := s
	claimed.NextRunAt = regularStamp
	if rerr := rt.restartService(ctx, s.AppName); rerr != nil {
		failures := s.Failures + 1
		retry := now.Add(restartBackoff(rt.restartDuration(ctx, KeyRestartScheduleBackoff, restartBackoffBounds), failures))
		if retry.After(regular) {
			retry = regular
		}
		if _, uerr := st.RecordAppRestartRun(ctx, claimed, stamp, store.AppRestartStatusFailed, "restart failed", failures, retry.UTC().Format(time.RFC3339)); uerr != nil {
			rerr = errors.Join(rerr, uerr)
		}
		rt.recordRestartScheduleAudit(ctx, "app.restart_schedule.failed", s.AppName, "restart failed")
		return false, rerr
	}
	if _, err := st.RecordAppRestartRun(ctx, claimed, stamp, store.AppRestartStatusSucceeded, "", 0, regularStamp); err != nil {
		rt.logger.Warn("api: restart schedule: record success failed", slog.String("name", s.AppName), slog.String("error", err.Error()))
	}
	if es := rt.appEvents(); es != nil {
		if err := es.AddAppEvent(ctx, store.AppEvent{AppName: s.AppName, Kind: store.AppEventRestart, Title: "Restarted by schedule", Actor: "scheduled restart"}); err != nil {
			rt.logger.Warn("api: restart schedule: record app event failed", slog.String("name", s.AppName), slog.String("error", err.Error()))
		}
	}
	rt.changes.Publish(changeApps, s.AppName)
	rt.nudgeReconciler()
	rt.recordRestartScheduleAudit(ctx, "app.restart_schedule.run", s.AppName, "")
	return true, nil
}

// backOffRestartRow pushes a row whose checks failed out by the failure
// backoff, so erroring rows cannot sort first every tick and starve healthy ones.
func (rt *Router) backOffRestartRow(ctx context.Context, st appRestartScheduleStore, s store.AppRestartSchedule, now, regular time.Time, cause error) error {
	failures := s.Failures + 1
	retry := now.Add(restartBackoff(rt.restartDuration(ctx, KeyRestartScheduleBackoff, restartBackoffBounds), failures))
	if !regular.IsZero() && retry.After(regular) {
		retry = regular
	}
	if _, err := st.RecordAppRestartRun(ctx, s, now.Format(time.RFC3339), store.AppRestartStatusFailed, "check failed", failures, retry.UTC().Format(time.RFC3339)); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
