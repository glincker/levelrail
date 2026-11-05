package store

import (
	"errors"
	"testing"
)

func TestUserPreferenceRoundTripAndSizeCap(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	const u = "pref_user"
	seedUserOnboardingUser(t, db, u)

	if _, found, err := db.GetUserPreference(ctx, u, "appearance"); err != nil || found {
		t.Fatalf("unset = found %v err %v", found, err)
	}
	if err := db.SetUserPreference(ctx, u, "appearance", `{"theme":"dark"}`, 64); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserPreference(ctx, u, "appearance", `{"theme":"light"}`, 64); err != nil {
		t.Fatal(err)
	}
	v, found, err := db.GetUserPreference(ctx, u, "appearance")
	if err != nil || !found || v != `{"theme":"light"}` {
		t.Fatalf("get = %q %v %v", v, found, err)
	}
	if err := db.SetUserPreference(ctx, u, "appearance", string(make([]byte, 65)), 64); !errors.Is(err, ErrUserPreferenceTooLarge) {
		t.Fatalf("oversize err = %v", err)
	}
	if err := db.DeleteUserPreference(ctx, u, "appearance"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := db.GetUserPreference(ctx, u, "appearance"); found {
		t.Fatal("still set after delete")
	}
}
