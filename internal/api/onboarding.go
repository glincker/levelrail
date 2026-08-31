package api

import (
	"context"
	"log/slog"
	"net/http"
)

// OnboardingStore is the store surface GET /api/v1/onboarding and POST
// /api/v1/onboarding/complete need: the single platform-wide row, always
// present, the same shape IngressSettingsStore has for its own row.
type OnboardingStore interface {
	GetOnboardingCompleted(ctx context.Context) (bool, error)
	MarkOnboardingCompleted(ctx context.Context) error
}

type onboardingStateResource struct {
	Completed bool `json:"completed"`
}

// handleGetOnboardingState handles GET /api/v1/onboarding.
func (rt *Router) handleGetOnboardingState(w http.ResponseWriter, r *http.Request) {
	completed, err := rt.onboarding.GetOnboardingCompleted(r.Context())
	if err != nil {
		rt.logger.Error("api: get onboarding state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, onboardingStateResource{Completed: completed})
}

// handleCompleteOnboarding handles POST /api/v1/onboarding/complete. One-way:
// there is no corresponding "uncomplete" route, matching
// MarkOnboardingCompleted's own doc comment.
func (rt *Router) handleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if err := rt.onboarding.MarkOnboardingCompleted(r.Context()); err != nil {
		rt.logger.Error("api: complete onboarding failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, onboardingStateResource{Completed: true})
}
