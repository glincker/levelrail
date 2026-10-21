package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AppSleepStore is the store surface the sleep routes need.
type AppSleepStore interface {
	SetAppSleepIdle(ctx context.Context, serviceName string, minutes int, now time.Time) error
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
	SetAppSleepHold(ctx context.Context, serviceName string, hold bool, now time.Time) error
	SetAppSleepOptions(ctx context.Context, serviceName string, ignorePaths []string, portWake *bool, now time.Time) error
	GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
	ListAppSleep(ctx context.Context) ([]store.AppSleep, error)
}

// WithWakeToken sets the secret the ingress wake route presents to the
// control plane. Without one, the wake hook answers 404 for everyone.
func WithWakeToken(token string) Option {
	return func(rt *Router) { rt.wakeToken = token }
}

type appSleepResource struct {
	Enabled     bool `json:"enabled"`
	IdleMinutes int  `json:"idle_minutes"`
	Sleeping    bool `json:"sleeping"`
	// Since is when the app went to sleep, present only while it is asleep.
	Since *time.Time `json:"since,omitempty"`
	// HoldRequests is function mode: a request that wakes the app waits for it.
	HoldRequests bool              `json:"hold_requests"`
	Blockers     []sleepItem       `json:"blockers"`
	Warnings     []sleepItem       `json:"warnings"`
	Eligible     bool              `json:"eligible"`
	Settings     sleepSettings     `json:"settings"`
	PortWake     sleepPortWake     `json:"port_wake"`
	Activity     sleepActivityView `json:"activity"`
	// ColdStartMsP50 is the median wake time from sleep history, null with no wakes.
	ColdStartMsP50 *int64 `json:"cold_start_ms_p50"`
	// WakeTiming summarises the app's recent wakes; samples is 0 with no history.
	WakeTiming wakeTimingResource `json:"wake_timing"`
	// IdleLimits are the allowed and default idle minutes on this server.
	IdleLimits sleepIdleLimits `json:"idle_limits"`
	// EstimatedMemoryFreedBytes is the app's memory limit, null when it has none.
	EstimatedMemoryFreedBytes *int64 `json:"estimated_memory_freed_bytes"`
}

type appSleepRequest struct {
	IdleMinutes  int       `json:"idle_minutes"`
	HoldRequests *bool     `json:"hold_requests,omitempty"`
	IgnorePaths  *[]string `json:"ignore_paths,omitempty"`
	PortWake     *bool     `json:"port_wake,omitempty"`
}

func toAppSleepResource(a store.AppSleep) appSleepResource {
	ignore := a.IgnorePaths
	if ignore == nil {
		ignore = []string{}
	}
	res := appSleepResource{
		Enabled: a.IdleMinutes > 0, IdleMinutes: a.IdleMinutes, Sleeping: a.Sleeping, HoldRequests: a.HoldRequests,
		Blockers: []sleepItem{}, Warnings: []sleepItem{}, PortWake: sleepPortWake{Ports: []int{}}, IdleLimits: currentIdleLimits(),
		Settings: sleepSettings{IdleMinutes: a.IdleMinutes, HoldRequests: a.HoldRequests, IgnorePaths: ignore, PortWake: a.PortWake},
	}
	if a.Sleeping {
		since := a.Since
		res.Since = &since
	}
	return res
}

func (rt *Router) handleGetAppSleep(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	a, err := rt.appSleep.GetAppSleep(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get app sleep", err, slog.String("name", name))
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeJSON(w, http.StatusOK, toAppSleepResource(a))
		return
	} else if err != nil {
		rt.internalError(w, "api: get app sleep: load app", err, slog.String("name", name))
		return
	}
	fleet, err := rt.sleepFleet(r.Context())
	if err != nil {
		rt.internalError(w, "api: get app sleep: load fleet", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.sleepResource(r.Context(), fleet, svc, a, time.Now()))
}

// handleSetAppSleep enables sleeping after idle_minutes without requests, or
// disables it with 0 (waking the app first if it is asleep).
func (rt *Router) handleSetAppSleep(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req appSleepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if lo, hi := sleepIdleBounds(); req.IdleMinutes != 0 && (req.IdleMinutes < lo || req.IdleMinutes > hi) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("idle_minutes must be 0 (off) or between %d and %d", lo, hi))
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app sleep: load app", err, slog.String("name", name))
		return
	}
	var ignorePaths []string
	if req.IgnorePaths != nil {
		var bad string
		if ignorePaths, bad = validateIgnorePaths(*req.IgnorePaths); bad != "" {
			writeError(w, http.StatusBadRequest, bad)
			return
		}
	}
	if req.IdleMinutes > 0 && !rt.sleepEnableAllowed(w, r, svc, req) {
		return
	}
	prev, _ := rt.appSleep.GetAppSleep(r.Context(), name)
	actor := rt.eventActor(r)
	if req.IdleMinutes == 0 {
		woke, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, time.Now())
		if err != nil {
			rt.internalError(w, "api: set app sleep: wake before disable", err, slog.String("name", name))
			return
		}
		if woke {
			rt.recordSleepEvent(r.Context(), appsleep.WokeEvent(name, store.SleepReasonManual, actor, prev.Since, time.Now()))
		}
	}
	if err := rt.appSleep.SetAppSleepIdle(r.Context(), name, req.IdleMinutes, time.Now()); err != nil {
		rt.internalError(w, "api: set app sleep", err, slog.String("name", name))
		return
	}
	switch {
	case req.IdleMinutes > 0 && prev.IdleMinutes != req.IdleMinutes:
		rt.recordSleepEvent(r.Context(), store.SleepEvent{ServiceName: name, Kind: store.SleepEventEnabled, At: time.Now(), Reason: store.SleepReasonAPI, IdleMinutes: req.IdleMinutes, Actor: actor})
	case req.IdleMinutes == 0 && prev.IdleMinutes > 0:
		rt.recordSleepEvent(r.Context(), store.SleepEvent{ServiceName: name, Kind: store.SleepEventDisabled, At: time.Now(), Reason: store.SleepReasonAPI, Actor: actor})
	}
	if req.HoldRequests != nil && req.IdleMinutes > 0 {
		if err := rt.appSleep.SetAppSleepHold(r.Context(), name, *req.HoldRequests, time.Now()); err != nil {
			rt.internalError(w, "api: set app sleep hold", err, slog.String("name", name))
			return
		}
	}
	if req.IdleMinutes > 0 && (ignorePaths != nil || req.PortWake != nil) {
		if err := rt.appSleep.SetAppSleepOptions(r.Context(), name, ignorePaths, req.PortWake, time.Now()); err != nil {
			rt.internalError(w, "api: set app sleep options", err, slog.String("name", name))
			return
		}
	}
	rt.syncPortWake(r.Context())
	rt.writeSleepResource(w, r, svc)
}

// sleepEnableAllowed refuses with 409 and the blockers when sleep cannot be
// turned on, judging the settings the request would leave in place.
func (rt *Router) sleepEnableAllowed(w http.ResponseWriter, r *http.Request, svc *store.DesiredService, req appSleepRequest) bool {
	setting, _ := rt.appSleep.GetAppSleep(r.Context(), svc.Name)
	setting.IdleMinutes = req.IdleMinutes
	if req.PortWake != nil {
		setting.PortWake = *req.PortWake
	}
	if req.HoldRequests != nil {
		setting.HoldRequests = *req.HoldRequests
	}
	fleet, err := rt.sleepFleet(r.Context())
	if err != nil {
		rt.internalError(w, "api: set app sleep: load fleet", err, slog.String("name", svc.Name))
		return false
	}
	blockers := appsleep.Evaluate(fleet.Facts(svc, setting, rt.sleepPortWake)).Blockers
	if len(blockers) == 0 {
		return true
	}
	writeJSON(w, http.StatusConflict, sleepRefusal{Error: "sleep when idle cannot be turned on: " + blockers[0].Title, Blockers: toSleepItems(blockers)})
	return false
}

// writeSleepResource answers with the app's full, freshly computed sleep state.
func (rt *Router) writeSleepResource(w http.ResponseWriter, r *http.Request, svc *store.DesiredService) {
	a, _ := rt.appSleep.GetAppSleep(r.Context(), svc.Name)
	fleet, err := rt.sleepFleet(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, toAppSleepResource(a))
		return
	}
	writeJSON(w, http.StatusOK, rt.sleepResource(r.Context(), fleet, svc, a, time.Now()))
}

func (rt *Router) syncPortWake(ctx context.Context) {
	if s, ok := rt.sleepPortWake.(appsleep.PortSyncer); ok {
		s.Sync(ctx)
	}
}

// handleWakeApp starts a sleeping app now.
func (rt *Router) handleWakeApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	prev, _ := rt.appSleep.GetAppSleep(r.Context(), name)
	start := time.Now()
	woke, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, start)
	if err != nil {
		rt.recordSleepEvent(r.Context(), store.SleepEvent{ServiceName: name, Kind: store.SleepEventWakeFailed, At: start, Reason: store.SleepReasonManual, Detail: err.Error(), Actor: rt.eventActor(r)})
		rt.internalError(w, "api: wake app", err, slog.String("name", name))
		return
	}
	if woke {
		id := rt.recordSleepEvent(r.Context(), appsleep.WokeEvent(name, store.SleepReasonManual, rt.eventActor(r), prev.Since, start))
		rt.watchWake(name, id, start, store.SleepReasonManual)
	}
	rt.syncPortWake(r.Context())
	if svc, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		rt.writeSleepResource(w, r, svc)
		return
	}
	a, _ := rt.appSleep.GetAppSleep(r.Context(), name)
	writeJSON(w, http.StatusOK, toAppSleepResource(a))
}

// handleWakeHook is what the ingress wake route proxies to when a request hits
// a sleeping app. It answers 404 unless the shared token matches, so it cannot
// be used to start apps from outside.
func (rt *Router) handleWakeHook(w http.ResponseWriter, r *http.Request) {
	got := r.Header.Get(ingress.WakeTokenHeader)
	guessKey := clientIP(r)
	if allowed, _ := rt.wakeTokenGuess.allow(guessKey); !allowed {
		http.NotFound(w, r)
		return
	}
	if rt.wakeToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(rt.wakeToken)) != 1 {
		rt.wakeTokenGuess.recordFailure(guessKey)
		http.NotFound(w, r)
		return
	}
	name := r.Header.Get(ingress.WakeAppHeader)
	w.Header().Set(ingress.WakeHeldHeader, "1")
	setting, _ := rt.appSleep.GetAppSleep(r.Context(), name)
	timing := rt.wakeTiming(r.Context(), name)
	cfg := appsleep.ConfigFromEnv(nil)
	hold := cfg.HoldTimeout(timing.ReadyP95Ms)
	if setting.HoldRequests {
		fnSettings := rt.loadFunctionSettings(r.Context(), name)
		lim := functions.LimitsFromEnv(nil)
		if t := lim.Timeout(fnSettings); t > 0 {
			hold = t
		}
		release, ok := rt.functionLimiter.Acquire(name, lim.Concurrency(fnSettings))
		if !ok {
			rt.logger.Warn("api: wake hook: function at max concurrency", slog.String("function", name))
			w.Header().Set("Retry-After", strconv.Itoa(wakeRetryAfterSeconds(timing)))
			writeError(w, http.StatusTooManyRequests, "function is at its concurrency limit")
			return
		}
		defer release()
	}
	start := time.Now()
	woke, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, start)
	var eventID int64
	switch {
	case err != nil:
		rt.logger.Error("api: wake hook", slog.String("app", name), slog.String("error", err.Error()))
		e := wakeRequestEvent(r, name, setting.Since, start)
		e.Kind, e.Detail = store.SleepEventWakeFailed, err.Error()
		rt.recordSleepEvent(r.Context(), e)
	case woke:
		eventID = rt.recordSleepEvent(r.Context(), wakeRequestEvent(r, name, setting.Since, start))
	}
	// A request that arrives while the wake is already running is held too, up
	// to what is left of the hold window, not answered with the waking page.
	wakeStart, remaining := start, hold
	if !setting.Sleeping && !setting.Since.IsZero() {
		wakeStart = setting.Since
		remaining = hold - start.Sub(setting.Since)
	}
	holding := setting.HoldRequests && err == nil && remaining > 0 && (setting.Sleeping || start.Sub(setting.Since) < hold)
	if holding {
		ready := rt.holdUntilReady(r.Context(), name, wakeStart, remaining, cfg.WakeProbeInterval)
		rt.finishWake(r.Context(), name, eventID, wakeStart, hold, ready, store.SleepReasonRequest)
		if ready {
			rt.markWakeReleased(r.Context(), eventID, wakeStart)
			if loc := wakeRedirectURL(r); loc != "" {
				http.Redirect(w, r, loc, http.StatusTemporaryRedirect)
				return
			}
		}
	} else {
		rt.watchWake(name, eventID, start, store.SleepReasonRequest)
	}
	rt.writeWakingResponse(w, r, name, timing)
}

// wakeIngressSettle lets the ingress route settle after the app is ready. A
// variable so tests can shorten it.
var wakeIngressSettle = 200 * time.Millisecond

// holdUntilReady waits up to timeout for a Ready=True condition on the app
// recorded after since (a Ready from before it slept does not count), then for
// an ingress reconcile pass recorded after that, so the route already points
// at the new container, then a brief settle. It checks every poll, the short
// wake interval, and returns on the first ready signal. false means it timed out.
func (rt *Router) holdUntilReady(ctx context.Context, name string, since time.Time, timeout, poll time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var readyAt time.Time
	for {
		if readyAt.IsZero() {
			readyAt = rt.readyConditionAfter(ctx, applicationControllerName(name), since)
		} else if !rt.readyConditionAfter(ctx, wakeIngressController, readyAt).IsZero() {
			select {
			case <-time.After(wakeIngressSettle):
				return true
			case <-ctx.Done():
				return false
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return false
		}
	}
}

// wakeIngressController is the ingress reconciler's condition name.
const wakeIngressController = "ingress"

// readyConditionAfter returns when controller last recorded a Ready=True
// condition if that is after since, else the zero time.
func (rt *Router) readyConditionAfter(ctx context.Context, controller string, since time.Time) time.Time {
	conds, err := rt.deploys.GetConditions(ctx, controller)
	if err != nil {
		return time.Time{}
	}
	for _, c := range conds {
		if c.Type == reconcile.ConditionTypeReady && c.Status == reconcile.ConditionTrue && c.LastTransitionTime.After(since) {
			return c.LastTransitionTime
		}
	}
	return time.Time{}
}

// wakeRedirectURL rebuilds the URL the client originally asked for. 307 keeps
// the method and body, so a POST to a sleeping function is replayed as is.
func wakeRedirectURL(r *http.Request) string {
	uri := r.Header.Get(ingress.WakeURIHeader)
	if uri == "" || !strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "//") {
		return ""
	}
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "http" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + uri
}

// handleSleepAppNow puts an app with sleep-when-idle on to sleep without
// waiting for the idle time.
func (rt *Router) handleSleepAppNow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: sleep app now: load app", err, slog.String("name", name))
		return
	}
	a, err := rt.appSleep.GetAppSleep(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: sleep app now: load setting", err, slog.String("name", name))
		return
	}
	if a.IdleMinutes == 0 {
		writeError(w, http.StatusConflict, "turn on sleep when idle for this app first")
		return
	}
	fleet, err := rt.sleepFleet(r.Context())
	if err != nil {
		rt.internalError(w, "api: sleep app now: load fleet", err, slog.String("name", name))
		return
	}
	if blockers := appsleep.Evaluate(fleet.Facts(svc, a, rt.sleepPortWake)).Blockers; len(blockers) > 0 {
		writeJSON(w, http.StatusConflict, sleepRefusal{Error: "this app cannot sleep: " + blockers[0].Title, Blockers: toSleepItems(blockers)})
		return
	}
	if svc.Suspended {
		writeError(w, http.StatusConflict, "the app is already stopped")
		return
	}
	if err := appsleep.Sleep(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, time.Now()); err != nil {
		rt.internalError(w, "api: sleep app now", err, slog.String("name", name))
		return
	}
	rt.recordSleepEvent(r.Context(), store.SleepEvent{ServiceName: name, Kind: store.SleepEventSlept, At: time.Now(), Reason: store.SleepReasonManual, IdleMinutes: a.IdleMinutes, Actor: rt.eventActor(r)})
	rt.writeSleepResource(w, r, svc)
}
