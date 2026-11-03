package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedUserOnboardingUser(t *testing.T, db *DB, id string) {
	t.Helper()
	u := User{ID: id, Email: id + "@example.com", DisplayName: id, CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func TestUserOnboardingTransitions(t *testing.T) {
	states := []UserOnboardingState{UserOnboardingNotStarted, UserOnboardingInProgress, UserOnboardingSnoozed, UserOnboardingCompleted, UserOnboardingDismissed}
	// path reaches each state from not_started through legal moves.
	path := map[UserOnboardingState][]UserOnboardingState{
		UserOnboardingNotStarted: nil,
		UserOnboardingInProgress: {UserOnboardingInProgress},
		UserOnboardingSnoozed:    {UserOnboardingInProgress, UserOnboardingSnoozed},
		UserOnboardingCompleted:  {UserOnboardingInProgress, UserOnboardingCompleted},
		UserOnboardingDismissed:  {UserOnboardingDismissed},
	}
	legal := map[[2]UserOnboardingState]bool{
		{UserOnboardingNotStarted, UserOnboardingInProgress}: true,
		{UserOnboardingNotStarted, UserOnboardingSnoozed}:    true,
		{UserOnboardingNotStarted, UserOnboardingDismissed}:  true,
		{UserOnboardingInProgress, UserOnboardingSnoozed}:    true,
		{UserOnboardingInProgress, UserOnboardingCompleted}:  true,
		{UserOnboardingInProgress, UserOnboardingDismissed}:  true,
		{UserOnboardingSnoozed, UserOnboardingInProgress}:    true,
		{UserOnboardingSnoozed, UserOnboardingDismissed}:     true,
		{UserOnboardingCompleted, UserOnboardingDismissed}:   true,
	}
	for _, from := range states {
		for _, to := range states {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				db := openTestDB(t)
				ctx := context.Background()
				seedUserOnboardingUser(t, db, "u1")
				for _, s := range path[from] {
					if _, err := db.TransitionUserOnboarding(ctx, "u1", s, ""); err != nil {
						t.Fatalf("setup to %s: %v", s, err)
					}
				}
				got, err := db.TransitionUserOnboarding(ctx, "u1", to, "")
				if from == to || legal[[2]UserOnboardingState{from, to}] {
					if err != nil {
						t.Fatalf("want ok, got %v", err)
					}
					if got.State != to {
						t.Fatalf("state = %s, want %s", got.State, to)
					}
					return
				}
				var inv *InvalidUserOnboardingTransitionError
				if !errors.Is(err, ErrInvalidUserOnboardingTransition) || !errors.As(err, &inv) {
					t.Fatalf("want invalid transition, got %v", err)
				}
				if cur, _ := db.GetUserOnboarding(ctx, "u1"); cur.State != from {
					t.Fatalf("state changed to %s after refused move", cur.State)
				}
			})
		}
	}
}

func TestUserOnboardingIdempotentAndStamps(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUserOnboardingUser(t, db, "u1")
	first, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingInProgress, "theme")
	if err != nil || first.StartedAt == nil || first.CurrentStep != "theme" || first.Version != UserOnboardingFlowVersion {
		t.Fatalf("first = %+v, %v", first, err)
	}
	again, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingInProgress, "")
	if err != nil || !again.StartedAt.Equal(*first.StartedAt) || again.CurrentStep != "theme" {
		t.Fatalf("repeat changed row: %+v, %v", again, err)
	}
	done, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingCompleted, "finish")
	if err != nil || done.CompletedAt == nil {
		t.Fatalf("complete = %+v, %v", done, err)
	}
	if _, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingInProgress, ""); !errors.Is(err, ErrInvalidUserOnboardingTransition) {
		t.Fatalf("backwards = %v", err)
	}
	if _, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingState("bogus"), ""); !errors.Is(err, ErrInvalidUserOnboardingTransition) {
		t.Fatalf("unknown state = %v", err)
	}
}

func TestUserOnboardingResetAndIsolation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedUserOnboardingUser(t, db, "u1")
	seedUserOnboardingUser(t, db, "u2")
	if _, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingDismissed, ""); err != nil {
		t.Fatal(err)
	}
	if o, _ := db.GetUserOnboarding(ctx, "u2"); o.State != UserOnboardingNotStarted {
		t.Fatalf("u2 = %s, want not_started", o.State)
	}
	if err := db.ResetUserOnboarding(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	o, _ := db.GetUserOnboarding(ctx, "u1")
	if o.State != UserOnboardingNotStarted || o.DismissedAt != nil {
		t.Fatalf("after reset = %+v", o)
	}
	if _, err := db.TransitionUserOnboarding(ctx, "u1", UserOnboardingInProgress, ""); err != nil {
		t.Fatalf("restart after reset: %v", err)
	}
	if err := db.ResetUserOnboarding(ctx, "nobody"); err != nil {
		t.Fatalf("reset unknown user: %v", err)
	}
}
