package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// OnboardingStore is the store surface the /api/v1/onboarding routes need.
type OnboardingStore interface {
	GetOnboardingState(ctx context.Context) (store.OnboardingState, error)
	MarkOnboardingCompleted(ctx context.Context) error
	SetOnboardingProgress(ctx context.Context, currentStep string, steps map[string]string) error
}

// onboardingSteps are the setup wizard's step ids, in order.
var onboardingSteps = []string{"server", "domain", "git", "app", "done"}

const (
	onboardingStepCompleted = "completed"
	onboardingStepSkipped   = "skipped"
)

type onboardingStateResource struct {
	Completed   bool              `json:"completed"`
	CurrentStep string            `json:"current_step"`
	Steps       map[string]string `json:"steps"`
}

type onboardingProgressRequest struct {
	CurrentStep string            `json:"current_step"`
	Steps       map[string]string `json:"steps"`
}

func toOnboardingStateResource(s store.OnboardingState) onboardingStateResource {
	steps := s.Steps
	if steps == nil {
		steps = map[string]string{}
	}
	return onboardingStateResource{Completed: s.Completed, CurrentStep: s.CurrentStep, Steps: steps}
}

func isOnboardingStep(id string) bool {
	for _, s := range onboardingSteps {
		if s == id {
			return true
		}
	}
	return false
}

func validateOnboardingProgress(req onboardingProgressRequest) error {
	if req.CurrentStep != "" && !isOnboardingStep(req.CurrentStep) {
		return fmt.Errorf("current_step %q is not a setup step", req.CurrentStep)
	}
	for id, status := range req.Steps {
		if !isOnboardingStep(id) {
			return fmt.Errorf("steps: %q is not a setup step", id)
		}
		if status != onboardingStepCompleted && status != onboardingStepSkipped {
			return fmt.Errorf("steps.%s must be %q or %q", id, onboardingStepCompleted, onboardingStepSkipped)
		}
	}
	return nil
}

func (rt *Router) writeOnboardingState(w http.ResponseWriter, r *http.Request) {
	state, err := rt.onboarding.GetOnboardingState(r.Context())
	if err != nil {
		rt.logger.Error("api: get onboarding state failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toOnboardingStateResource(state))
}

// handleGetOnboardingState handles GET /api/v1/onboarding.
func (rt *Router) handleGetOnboardingState(w http.ResponseWriter, r *http.Request) {
	rt.writeOnboardingState(w, r)
}

// handleCompleteOnboarding handles POST /api/v1/onboarding/complete.
func (rt *Router) handleCompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	if err := rt.onboarding.MarkOnboardingCompleted(r.Context()); err != nil {
		rt.logger.Error("api: complete onboarding failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.writeOnboardingState(w, r)
}

// handleUpdateOnboardingProgress handles PUT /api/v1/onboarding/progress.
func (rt *Router) handleUpdateOnboardingProgress(w http.ResponseWriter, r *http.Request) {
	var req onboardingProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateOnboardingProgress(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.onboarding.SetOnboardingProgress(r.Context(), req.CurrentStep, req.Steps); err != nil {
		rt.logger.Error("api: update onboarding progress failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.writeOnboardingState(w, r)
}
