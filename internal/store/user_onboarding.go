package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// UserOnboardingState is where one user stands in the first-run flow.
type UserOnboardingState string

// Onboarding states. Dismissed is the skip state; Reset returns to NotStarted.
const (
	UserOnboardingNotStarted UserOnboardingState = "not_started"
	UserOnboardingInProgress UserOnboardingState = "in_progress"
	UserOnboardingSnoozed    UserOnboardingState = "snoozed"
	UserOnboardingCompleted  UserOnboardingState = "completed"
	UserOnboardingDismissed  UserOnboardingState = "dismissed"
)

// UserOnboardingFlowVersion lets a later flow redesign tell old rows apart.
const UserOnboardingFlowVersion = 1

// userOnboardingNext lists the forward moves; dismissing is allowed from every state.
var userOnboardingNext = map[UserOnboardingState][]UserOnboardingState{
	UserOnboardingNotStarted: {UserOnboardingInProgress, UserOnboardingDismissed},
	UserOnboardingInProgress: {UserOnboardingSnoozed, UserOnboardingCompleted, UserOnboardingDismissed},
	UserOnboardingSnoozed:    {UserOnboardingInProgress, UserOnboardingDismissed},
	UserOnboardingCompleted:  {UserOnboardingDismissed},
	UserOnboardingDismissed:  {},
}

// ErrInvalidUserOnboardingTransition matches every InvalidUserOnboardingTransitionError.
var ErrInvalidUserOnboardingTransition = errors.New("store: invalid onboarding transition")

// InvalidUserOnboardingTransitionError names the refused move.
type InvalidUserOnboardingTransitionError struct {
	From, To UserOnboardingState
}

func (e *InvalidUserOnboardingTransitionError) Error() string {
	return fmt.Sprintf("store: onboarding cannot move from %q to %q", e.From, e.To)
}

// Is lets errors.Is(err, ErrInvalidUserOnboardingTransition) match.
func (e *InvalidUserOnboardingTransitionError) Is(target error) bool {
	return target == ErrInvalidUserOnboardingTransition
}

// UserOnboarding is one user's stored flow position.
type UserOnboarding struct {
	UserID      string
	State       UserOnboardingState
	CurrentStep string
	Version     int
	StartedAt   *time.Time
	CompletedAt *time.Time
	DismissedAt *time.Time
	UpdatedAt   time.Time
}

// UserOnboardingTransitionAllowed reports whether from may move to to (same state counts as allowed).
func UserOnboardingTransitionAllowed(from, to UserOnboardingState) bool {
	if from == to {
		return true
	}
	for _, n := range userOnboardingNext[from] {
		if n == to {
			return true
		}
	}
	return false
}

// GetUserOnboarding returns the stored row, or a not_started value when the user has none.
func (db *DB) GetUserOnboarding(ctx context.Context, userID string) (UserOnboarding, error) {
	return getUserOnboarding(ctx, db.DB, userID)
}

type userOnboardingQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getUserOnboarding(ctx context.Context, q userOnboardingQuerier, userID string) (UserOnboarding, error) {
	o := UserOnboarding{UserID: userID, State: UserOnboardingNotStarted, CurrentStep: "welcome", Version: UserOnboardingFlowVersion}
	var started, completed, dismissed, updated string
	err := q.QueryRowContext(ctx, `
		SELECT state, current_step, flow_version, started_at, completed_at, dismissed_at, updated_at
		FROM user_onboarding WHERE user_id = ?`, userID).
		Scan(&o.State, &o.CurrentStep, &o.Version, &started, &completed, &dismissed, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return o, nil
	}
	if err != nil {
		return o, fmt.Errorf("store: get onboarding for %q: %w", userID, err)
	}
	o.StartedAt, _ = parseTimeOrNil(started)
	o.CompletedAt, _ = parseTimeOrNil(completed)
	o.DismissedAt, _ = parseTimeOrNil(dismissed)
	o.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return o, nil
}

// TransitionUserOnboarding moves the user to state and records step (empty keeps the current one).
// Repeating the current state is an idempotent upsert; an illegal move returns InvalidUserOnboardingTransitionError.
func (db *DB) TransitionUserOnboarding(ctx context.Context, userID string, to UserOnboardingState, step string) (UserOnboarding, error) {
	if _, ok := userOnboardingNext[to]; !ok {
		return UserOnboarding{}, &InvalidUserOnboardingTransitionError{To: to}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return UserOnboarding{}, fmt.Errorf("store: begin onboarding transition for %q: %w", userID, err)
	}
	defer func() { _ = tx.Rollback() }()

	cur, err := getUserOnboarding(ctx, tx, userID)
	if err != nil {
		return cur, err
	}
	if !UserOnboardingTransitionAllowed(cur.State, to) {
		return cur, &InvalidUserOnboardingTransitionError{From: cur.State, To: to}
	}
	now := time.Now().UTC()
	stamp := formatTime(now)
	if step == "" {
		step = cur.CurrentStep
	}
	started, completed, dismissed := stampOrEmpty(cur.StartedAt), stampOrEmpty(cur.CompletedAt), stampOrEmpty(cur.DismissedAt)
	if cur.State != to {
		switch to {
		case UserOnboardingInProgress:
			if started == "" {
				started = stamp
			}
		case UserOnboardingCompleted:
			completed = stamp
		case UserOnboardingDismissed:
			dismissed = stamp
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_onboarding (user_id, state, current_step, flow_version, started_at, completed_at, dismissed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET state = excluded.state, current_step = excluded.current_step,
			flow_version = excluded.flow_version, started_at = excluded.started_at,
			completed_at = excluded.completed_at, dismissed_at = excluded.dismissed_at, updated_at = excluded.updated_at`,
		userID, string(to), step, UserOnboardingFlowVersion, started, completed, dismissed, stamp); err != nil {
		return cur, fmt.Errorf("store: save onboarding for %q: %w", userID, err)
	}
	if err := tx.Commit(); err != nil {
		return cur, fmt.Errorf("store: commit onboarding for %q: %w", userID, err)
	}
	return db.GetUserOnboarding(ctx, userID)
}

// ResetUserOnboarding returns the user to not_started and clears the timestamps (admin restart).
func (db *DB) ResetUserOnboarding(ctx context.Context, userID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_onboarding WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: reset onboarding for %q: %w", userID, err)
	}
	return nil
}

func stampOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
