package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	minSleepIdleMinutes = 5
	maxSleepIdleMinutes = 7 * 24 * 60
)

// AppSleepStore is the store surface the sleep routes need.
type AppSleepStore interface {
	SetAppSleepIdle(ctx context.Context, serviceName string, minutes int, now time.Time) error
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
	SetAppSleepHold(ctx context.Context, serviceName string, hold bool, now time.Time) error
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
	// HoldRequests is function mode: a request that wakes the app waits for it.
	HoldRequests bool `json:"hold_requests"`
}

type appSleepRequest struct {
	IdleMinutes  int   `json:"idle_minutes"`
	HoldRequests *bool `json:"hold_requests,omitempty"`
}

func toAppSleepResource(a store.AppSleep) appSleepResource {
	return appSleepResource{Enabled: a.IdleMinutes > 0, IdleMinutes: a.IdleMinutes, Sleeping: a.Sleeping, HoldRequests: a.HoldRequests}
}

func (rt *Router) handleGetAppSleep(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	a, err := rt.appSleep.GetAppSleep(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get app sleep", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toAppSleepResource(a))
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
	if req.IdleMinutes != 0 && (req.IdleMinutes < minSleepIdleMinutes || req.IdleMinutes > maxSleepIdleMinutes) {
		writeError(w, http.StatusBadRequest, "idle_minutes must be 0 (off) or between 5 and 10080")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app sleep: load app", err, slog.String("name", name))
		return
	}
	if req.IdleMinutes == 0 {
		if _, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, time.Now()); err != nil {
			rt.internalError(w, "api: set app sleep: wake before disable", err, slog.String("name", name))
			return
		}
	}
	if err := rt.appSleep.SetAppSleepIdle(r.Context(), name, req.IdleMinutes, time.Now()); err != nil {
		rt.internalError(w, "api: set app sleep", err, slog.String("name", name))
		return
	}
	if req.HoldRequests != nil && req.IdleMinutes > 0 {
		if err := rt.appSleep.SetAppSleepHold(r.Context(), name, *req.HoldRequests, time.Now()); err != nil {
			rt.internalError(w, "api: set app sleep hold", err, slog.String("name", name))
			return
		}
	}
	a, _ := rt.appSleep.GetAppSleep(r.Context(), name)
	writeJSON(w, http.StatusOK, toAppSleepResource(a))
}

// handleWakeApp starts a sleeping app now.
func (rt *Router) handleWakeApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if _, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, time.Now()); err != nil {
		rt.internalError(w, "api: wake app", err, slog.String("name", name))
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
	if rt.wakeToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(rt.wakeToken)) != 1 {
		http.NotFound(w, r)
		return
	}
	name := r.Header.Get(ingress.WakeAppHeader)
	setting, _ := rt.appSleep.GetAppSleep(r.Context(), name)
	if _, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, time.Now()); err != nil {
		rt.logger.Error("api: wake hook", slog.String("app", name), slog.String("error", err.Error()))
	}
	if setting.HoldRequests && setting.Sleeping && rt.holdUntilReady(r.Context(), name, time.Now()) {
		if loc := wakeRedirectURL(r); loc != "" {
			http.Redirect(w, r, loc, http.StatusTemporaryRedirect)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Variables, not constants, so tests can shorten them.
var (
	wakeHoldTimeout   = 30 * time.Second
	wakeHoldPoll      = 250 * time.Millisecond
	wakeIngressSettle = 200 * time.Millisecond
)

// holdUntilReady waits for a Ready=True condition on the app recorded after
// since (a Ready from before it slept does not count), then for an ingress
// reconcile pass recorded after that, so the route already points at the new
// container, then a brief settle. false means it timed out.
func (rt *Router) holdUntilReady(ctx context.Context, name string, since time.Time) bool {
	ctx, cancel := context.WithTimeout(ctx, wakeHoldTimeout)
	defer cancel()
	ticker := time.NewTicker(wakeHoldPoll)
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
