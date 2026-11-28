package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/store"
)

// capSleeper stops and starts capped apps through the sleep machinery. Resume
// bypasses appsleep.Wake on purpose: that path refuses to wake a capped app.
type capSleeper struct{ rt *Router }

func (s capSleeper) Pause(ctx context.Context, app string) error {
	now := time.Now()
	// An app without a sleep setting has no row to mark; the stop still lands.
	_ = s.rt.appSleep.SetAppSleeping(ctx, app, true, now)
	if err := s.rt.apps.UpdateServiceSuspended(ctx, app, true); err != nil {
		_ = s.rt.appSleep.SetAppSleeping(ctx, app, false, now)
		return err
	}
	s.rt.nudgeReconciler()
	s.rt.recordSleepEvent(ctx, store.SleepEvent{ServiceName: app, Kind: store.SleepEventSlept, At: now, Reason: store.SleepReasonCapped, Actor: "spend-cap-controller"})
	return nil
}

func (s capSleeper) Resume(ctx context.Context, app string) error {
	now := time.Now()
	prev, _ := s.rt.appSleep.GetAppSleep(ctx, app)
	_ = s.rt.appSleep.SetAppSleeping(ctx, app, false, now)
	if err := s.rt.apps.UpdateServiceSuspended(ctx, app, false); err != nil {
		return err
	}
	s.rt.nudgeReconciler()
	s.rt.recordSleepEvent(ctx, appsleep.WokeEvent(app, store.SleepReasonCapped, "spend-cap-controller", prev.Since, now))
	return nil
}

func (rt *Router) spendPauseStops(ctx context.Context) bool {
	return rt.runtimeSettings != nil && rt.runtimeSettings.Bool(ctx, runtimesettings.KeySpendPauseStopsApps)
}

// spendCapped reports whether a spend cap holds name stopped.
func (rt *Router) spendCapped(ctx context.Context, name string) bool {
	c, ok := rt.appSleep.(interface {
		SpendCapped(ctx context.Context, name string) (bool, error)
	})
	if !ok {
		return false
	}
	capped, err := c.SpendCapped(ctx, name)
	return err == nil && capped
}

type spendCapPausedResource struct {
	App     string    `json:"app"`
	Period  string    `json:"period"`
	Pct     float64   `json:"pct"`
	Since   time.Time `json:"since"`
	Message string    `json:"message"`
}

// handleListSpendCapPaused handles GET /api/v1/costs/capped-apps.
func (rt *Router) handleListSpendCapPaused(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.apps.(interface {
		ListSpendCapPaused(ctx context.Context) ([]spendcap.State, error)
	})
	if !ok {
		writeJSON(w, http.StatusOK, []spendCapPausedResource{})
		return
	}
	rows, err := st.ListSpendCapPaused(r.Context())
	if err != nil {
		rt.internalError(w, "api: list cap paused apps", err)
		return
	}
	out := make([]spendCapPausedResource, 0, len(rows))
	for _, s := range rows {
		out = append(out, spendCapPausedResource{App: s.App, Period: s.Period, Pct: s.Pct, Since: s.UpdatedAt, Message: "stopped by its spend cap until the limit resets or is raised"})
	}
	writeJSON(w, http.StatusOK, out)
}

type spendResumeRequest struct {
	Reason string `json:"reason"`
}

// handleSpendCapResume handles POST /api/v1/apps/{name}/budget/resume: start
// a cap-stopped app by hand, audited with a reason.
func (rt *Router) handleSpendCapResume(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c := rt.spendCaps()
	if c == nil {
		writeError(w, http.StatusNotImplemented, "spend caps are not available")
		return
	}
	var req spendResumeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || len(reason) > spendcap.MaxReasonLen {
		writeError(w, http.StatusBadRequest, "a reason is required and must be short")
		return
	}
	err := c.ResumeNow(r.Context(), name, rt.eventActor(r), reason)
	switch {
	case errors.Is(err, spendcap.ErrNotPaused):
		writeError(w, http.StatusConflict, "app is not stopped by a spend cap")
		return
	case err != nil:
		rt.internalError(w, "api: resume capped app", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"app": name, "status": "resumed"})
}
