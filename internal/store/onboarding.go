package store

import (
	"context"
	"fmt"
)

// GetOnboardingCompleted returns whether the first-run onboarding flow has
// been completed or dismissed. Always succeeds against a migrated database:
// migrations/0067_onboarding_state.sql seeds the single row (id = 1), the
// same "no not-found case" shape GetIngressSettings already has.
func (db *DB) GetOnboardingCompleted(ctx context.Context) (bool, error) {
	var completed int
	err := db.QueryRowContext(ctx, `
		SELECT completed FROM onboarding_state WHERE id = 1
	`).Scan(&completed)
	if err != nil {
		return false, fmt.Errorf("store: get onboarding state: %w", err)
	}
	return completed != 0, nil
}

// MarkOnboardingCompleted sets onboarding_state.completed to true. One-way:
// nothing in this codebase ever needs to un-complete it.
func (db *DB) MarkOnboardingCompleted(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `
		UPDATE onboarding_state SET completed = 1 WHERE id = 1
	`)
	if err != nil {
		return fmt.Errorf("store: mark onboarding completed: %w", err)
	}
	return nil
}
