package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/ingress"
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
}

type appSleepRequest struct {
	IdleMinutes int `json:"idle_minutes"`
}

func toAppSleepResource(a store.AppSleep) appSleepResource {
	return appSleepResource{Enabled: a.IdleMinutes > 0, IdleMinutes: a.IdleMinutes, Sleeping: a.Sleeping}
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
	if _, err := appsleep.Wake(r.Context(), rt.appSleep, rt.apps, rt.reconcileNudger, name, time.Now()); err != nil {
		rt.logger.Error("api: wake hook", slog.String("app", name), slog.String("error", err.Error()))
	}
	w.WriteHeader(http.StatusNoContent)
}
