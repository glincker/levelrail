package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/setupguide"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	onboardingEventUpdated = "onboarding.updated"
	onboardingEventReset   = "onboarding.reset"
	onboardingStepMaxLen   = 64
)

// userOnboardingStore is the slice of the store the per-user onboarding routes need.
type userOnboardingStore interface {
	GetUserOnboarding(ctx context.Context, userID string) (store.UserOnboarding, error)
	TransitionUserOnboarding(ctx context.Context, userID string, to store.UserOnboardingState, step string) (store.UserOnboarding, error)
	ResetUserOnboarding(ctx context.Context, userID string) error
}

// meOnboardingResource is the caller's own first-run flow position plus install progress.
type meOnboardingResource struct {
	State         string     `json:"state"`
	CurrentStep   string     `json:"current_step"`
	FlowVersion   int        `json:"flow_version"`
	Enabled       bool       `json:"enabled"`
	StartedAt     *time.Time `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	DismissedAt   *time.Time `json:"dismissed_at"`
	Milestone     string     `json:"milestone"`
	NextMilestone string     `json:"next_milestone"`
	RestoreDue    bool       `json:"restore_due"`
}

type meOnboardingRequest struct {
	State       string `json:"state"`
	CurrentStep string `json:"current_step"`
}

func (rt *Router) userOnboardingDB() userOnboardingStore {
	for _, c := range []any{rt.policies, rt.apps, rt.tokens, rt.auditLog} {
		if s, ok := c.(userOnboardingStore); ok {
			return s
		}
	}
	return nil
}

func (rt *Router) onboardingEnabled(r *http.Request) bool {
	if rt.runtimeSettings == nil {
		return true
	}
	return rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyOnboardingEnabled)
}

// meOnboardingBody builds the response with one setup guide derive for the milestone.
func (rt *Router) meOnboardingBody(r *http.Request, o store.UserOnboarding) meOnboardingResource {
	out := meOnboardingResource{
		State: string(o.State), CurrentStep: o.CurrentStep, FlowVersion: o.Version, Enabled: rt.onboardingEnabled(r),
		StartedAt: o.StartedAt, CompletedAt: o.CompletedAt, DismissedAt: o.DismissedAt,
	}
	if rt.setupGuide == nil {
		return out
	}
	state, err := rt.setupGuide.GetSetupGuideState(r.Context())
	if err != nil {
		return out
	}
	in := rt.collectSetupGuideInput(r)
	in.Hidden = state.Hidden
	in.Overrides = make(map[setupguide.StepID]string, len(state.Overrides))
	for id, v := range state.Overrides {
		in.Overrides[setupguide.StepID(id)] = v
	}
	g := setupguide.Derive(in)
	out.Milestone, out.NextMilestone = string(g.Milestone), string(g.NextMilestone)
	out.RestoreDue = rt.restoreReminderDue(r, o, in)
	return out
}

// restoreReminderDue is true once the user is past the reminder window with a backup but no verified restore.
func (rt *Router) restoreReminderDue(r *http.Request, o store.UserOnboarding, in setupguide.Input) bool {
	if rt.runtimeSettings == nil || o.StartedAt == nil {
		return false
	}
	days := rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyOnboardingRestoreReminderDays)
	if days <= 0 || in.ControlPlaneBackups == 0 || in.VerifiedBackups > 0 {
		return false
	}
	return time.Since(*o.StartedAt) >= time.Duration(days)*24*time.Hour
}

func (rt *Router) onboardingStoreOr501(w http.ResponseWriter) (userOnboardingStore, bool) {
	db := rt.userOnboardingDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "onboarding is not available on this server")
		return nil, false
	}
	return db, true
}

// handleGetMeOnboarding handles GET /api/v1/me/onboarding: the caller's own flow state.
func (rt *Router) handleGetMeOnboarding(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.accountCaller(w, r, AbilityRead)
	if !ok {
		return
	}
	db, ok := rt.onboardingStoreOr501(w)
	if !ok {
		return
	}
	o, err := db.GetUserOnboarding(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: get onboarding failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.meOnboardingBody(r, o))
}

// handlePutMeOnboarding handles PUT /api/v1/me/onboarding: moves the caller's own flow state.
func (rt *Router) handlePutMeOnboarding(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWrite)
	if !ok {
		return
	}
	db, ok := rt.onboardingStoreOr501(w)
	if !ok {
		return
	}
	var req meOnboardingRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	to := store.UserOnboardingState(req.State)
	switch to {
	case store.UserOnboardingNotStarted, store.UserOnboardingInProgress, store.UserOnboardingSnoozed,
		store.UserOnboardingCompleted, store.UserOnboardingDismissed:
	default:
		writeError(w, http.StatusBadRequest, "state must be not_started, in_progress, snoozed, completed or dismissed")
		return
	}
	if len(req.CurrentStep) > onboardingStepMaxLen {
		writeError(w, http.StatusBadRequest, "current_step is too long")
		return
	}
	if to != store.UserOnboardingDismissed && !rt.onboardingEnabled(r) {
		writeError(w, http.StatusForbidden, "onboarding is turned off on this server")
		return
	}
	o, err := db.TransitionUserOnboarding(r.Context(), userID, to, req.CurrentStep)
	if errors.Is(err, store.ErrInvalidUserOnboardingTransition) {
		writeJSON(w, http.StatusConflict, apiError{Error: err.Error(), Code: "invalid_transition", RequestID: w.Header().Get(requestIDHeader)})
		return
	}
	if err != nil {
		rt.internalError(w, "api: update onboarding failed", err)
		return
	}
	rt.recordAccountAction(r, userID, viaSession, onboardingEventUpdated, http.StatusOK)
	writeJSON(w, http.StatusOK, rt.meOnboardingBody(r, o))
}

// handleResetMeOnboarding handles POST /api/v1/me/onboarding/reset: restarts the caller's own flow.
func (rt *Router) handleResetMeOnboarding(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWrite)
	if !ok {
		return
	}
	db, ok := rt.onboardingStoreOr501(w)
	if !ok {
		return
	}
	if err := db.ResetUserOnboarding(r.Context(), userID); err != nil {
		rt.internalError(w, "api: reset onboarding failed", err)
		return
	}
	o, err := db.GetUserOnboarding(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: get onboarding after reset failed", err)
		return
	}
	rt.recordAccountAction(r, userID, viaSession, onboardingEventReset, http.StatusOK)
	writeJSON(w, http.StatusOK, rt.meOnboardingBody(r, o))
}
