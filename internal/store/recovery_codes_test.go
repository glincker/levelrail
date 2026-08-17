package store

import (
	"context"
	"testing"
	"time"
)

func seedUserForRecoveryCodes(t *testing.T, db *DB) string {
	t.Helper()
	ctx := context.Background()
	u := User{ID: "user_1", Email: "a@example.com", DisplayName: "A", CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return u.ID
}

func TestReplaceUserRecoveryCodes_AndConsume(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForRecoveryCodes(t, db)

	hashes := []string{"hash-a", "hash-b", "hash-c"}
	if err := db.ReplaceUserRecoveryCodes(ctx, userID, hashes); err != nil {
		t.Fatalf("ReplaceUserRecoveryCodes() error = %v", err)
	}

	n, err := db.CountUnusedUserRecoveryCodes(ctx, userID)
	if err != nil {
		t.Fatalf("CountUnusedUserRecoveryCodes() error = %v", err)
	}
	if n != 3 {
		t.Fatalf("CountUnusedUserRecoveryCodes() = %d, want 3", n)
	}

	ok, err := db.ConsumeUserRecoveryCode(ctx, userID, "hash-a")
	if err != nil {
		t.Fatalf("ConsumeUserRecoveryCode() error = %v", err)
	}
	if !ok {
		t.Fatal("ConsumeUserRecoveryCode() = false for a fresh, valid hash, want true")
	}

	n, err = db.CountUnusedUserRecoveryCodes(ctx, userID)
	if err != nil {
		t.Fatalf("CountUnusedUserRecoveryCodes() error = %v", err)
	}
	if n != 2 {
		t.Fatalf("CountUnusedUserRecoveryCodes() after consume = %d, want 2", n)
	}

	// Consuming the same code twice must fail the second time: single use.
	ok, err = db.ConsumeUserRecoveryCode(ctx, userID, "hash-a")
	if err != nil {
		t.Fatalf("ConsumeUserRecoveryCode() second call error = %v", err)
	}
	if ok {
		t.Error("ConsumeUserRecoveryCode() = true on a second attempt, want false")
	}

	// An unknown hash must also fail, indistinguishably from "already used".
	ok, err = db.ConsumeUserRecoveryCode(ctx, userID, "hash-does-not-exist")
	if err != nil {
		t.Fatalf("ConsumeUserRecoveryCode() unknown hash error = %v", err)
	}
	if ok {
		t.Error("ConsumeUserRecoveryCode() = true for an unknown hash, want false")
	}
}

func TestReplaceUserRecoveryCodes_ReplacesPriorSet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForRecoveryCodes(t, db)

	if err := db.ReplaceUserRecoveryCodes(ctx, userID, []string{"old-a", "old-b"}); err != nil {
		t.Fatalf("first ReplaceUserRecoveryCodes() error = %v", err)
	}
	if err := db.ReplaceUserRecoveryCodes(ctx, userID, []string{"new-a", "new-b", "new-c"}); err != nil {
		t.Fatalf("second ReplaceUserRecoveryCodes() error = %v", err)
	}

	n, err := db.CountUnusedUserRecoveryCodes(ctx, userID)
	if err != nil {
		t.Fatalf("CountUnusedUserRecoveryCodes() error = %v", err)
	}
	if n != 3 {
		t.Fatalf("CountUnusedUserRecoveryCodes() = %d, want 3 (old set replaced, not merged)", n)
	}

	// The old hash from before the regenerate must no longer validate.
	ok, err := db.ConsumeUserRecoveryCode(ctx, userID, "old-a")
	if err != nil {
		t.Fatalf("ConsumeUserRecoveryCode() error = %v", err)
	}
	if ok {
		t.Error("ConsumeUserRecoveryCode() = true for a code from the replaced set, want false")
	}
}

func TestDeleteUserRecoveryCodes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForRecoveryCodes(t, db)

	if err := db.ReplaceUserRecoveryCodes(ctx, userID, []string{"hash-a", "hash-b"}); err != nil {
		t.Fatalf("ReplaceUserRecoveryCodes() error = %v", err)
	}
	if err := db.DeleteUserRecoveryCodes(ctx, userID); err != nil {
		t.Fatalf("DeleteUserRecoveryCodes() error = %v", err)
	}

	n, err := db.CountUnusedUserRecoveryCodes(ctx, userID)
	if err != nil {
		t.Fatalf("CountUnusedUserRecoveryCodes() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("CountUnusedUserRecoveryCodes() after delete = %d, want 0", n)
	}
}

func TestUserTOTPSecretsKey_ParameterizedByUserID(t *testing.T) {
	a := UserTOTPSecretsKey("user_1")
	b := UserTOTPSecretsKey("user_2")
	if a == b {
		t.Fatalf("UserTOTPSecretsKey() returned the same key for different users: %q", a)
	}
	if a != "user-totp/user_1" {
		t.Errorf("UserTOTPSecretsKey(user_1) = %q, want %q", a, "user-totp/user_1")
	}
}
