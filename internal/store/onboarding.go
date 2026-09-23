package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// OnboardingState is the single platform-wide setup wizard row.
type OnboardingState struct {
	Completed   bool
	CurrentStep string
	// Steps maps a wizard step id to "completed" or "skipped".
	Steps map[string]string
}

// GetOnboardingState returns the setup wizard row seeded by migration 0067.
func (db *DB) GetOnboardingState(ctx context.Context) (OnboardingState, error) {
	var (
		completed int
		state     OnboardingState
		steps     string
	)
	err := db.QueryRowContext(ctx, `
		SELECT completed, current_step, step_status FROM onboarding_state WHERE id = 1
	`).Scan(&completed, &state.CurrentStep, &steps)
	if err != nil {
		return OnboardingState{}, fmt.Errorf("store: get onboarding state: %w", err)
	}
	state.Completed = completed != 0
	state.Steps = map[string]string{}
	if err := json.Unmarshal([]byte(steps), &state.Steps); err != nil {
		return OnboardingState{}, fmt.Errorf("store: decode onboarding step status: %w", err)
	}
	return state, nil
}

// GetOnboardingCompleted returns whether the setup wizard has been completed or dismissed.
func (db *DB) GetOnboardingCompleted(ctx context.Context) (bool, error) {
	state, err := db.GetOnboardingState(ctx)
	if err != nil {
		return false, err
	}
	return state.Completed, nil
}

// MarkOnboardingCompleted sets onboarding_state.completed to true; it is never unset.
func (db *DB) MarkOnboardingCompleted(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `
		UPDATE onboarding_state SET completed = 1 WHERE id = 1
	`)
	if err != nil {
		return fmt.Errorf("store: mark onboarding completed: %w", err)
	}
	return nil
}

// SetOnboardingProgress stores the wizard's current step and per-step status.
func (db *DB) SetOnboardingProgress(ctx context.Context, currentStep string, steps map[string]string) error {
	if steps == nil {
		steps = map[string]string{}
	}
	encoded, err := json.Marshal(steps)
	if err != nil {
		return fmt.Errorf("store: encode onboarding step status: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE onboarding_state SET current_step = ?, step_status = ? WHERE id = 1
	`, currentStep, string(encoded))
	if err != nil {
		return fmt.Errorf("store: set onboarding progress: %w", err)
	}
	return nil
}
