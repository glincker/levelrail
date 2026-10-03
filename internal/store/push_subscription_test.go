package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedUserForPushSubscriptions(t *testing.T, db *DB, id, email string) string {
	t.Helper()
	u := User{ID: id, Email: email, DisplayName: email, CreatedAt: time.Now().UTC()}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return u.ID
}

func TestSavePushSubscription_ThenListForUser(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForPushSubscriptions(t, db, "user_1", "a@example.com")

	s := PushSubscription{
		ID: "push_1", UserID: userID, Endpoint: "https://push.example.com/abc",
		P256dh: "p256dh-key", Auth: "auth-secret", UserAgent: "Mozilla/5.0", CreatedAt: time.Now().UTC(),
	}
	if err := db.SavePushSubscription(ctx, s); err != nil {
		t.Fatalf("SavePushSubscription() error = %v", err)
	}

	rows, err := db.ListPushSubscriptionsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListPushSubscriptionsForUser() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Endpoint != s.Endpoint || rows[0].P256dh != s.P256dh || rows[0].Auth != s.Auth {
		t.Errorf("row = %+v, want it to round-trip the saved fields", rows[0])
	}
}

func TestSavePushSubscription_SameEndpoint_Upserts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForPushSubscriptions(t, db, "user_1", "a@example.com")

	endpoint := "https://push.example.com/abc"
	if err := db.SavePushSubscription(ctx, PushSubscription{
		ID: "push_1", UserID: userID, Endpoint: endpoint, P256dh: "old-key", Auth: "old-auth", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SavePushSubscription() error = %v", err)
	}
	if err := db.SavePushSubscription(ctx, PushSubscription{
		ID: "push_2", UserID: userID, Endpoint: endpoint, P256dh: "new-key", Auth: "new-auth", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SavePushSubscription() re-subscribe error = %v", err)
	}

	rows, err := db.ListPushSubscriptionsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListPushSubscriptionsForUser() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (re-subscribing the same endpoint should upsert, not duplicate)", len(rows))
	}
	if rows[0].P256dh != "new-key" {
		t.Errorf("P256dh = %q, want the latest subscribe call's key to win", rows[0].P256dh)
	}
}

func TestListPushSubscriptions_AcrossUsers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	user1 := seedUserForPushSubscriptions(t, db, "user_1", "a@example.com")
	user2 := seedUserForPushSubscriptions(t, db, "user_2", "b@example.com")

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: user1, Endpoint: "https://a", P256dh: "k1", Auth: "a1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_2", UserID: user2, Endpoint: "https://b", P256dh: "k2", Auth: "a2", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	all, err := db.ListPushSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListPushSubscriptions() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d rows, want 2 (every user's subscriptions)", len(all))
	}
}

func TestDeletePushSubscription_WrongUser_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	owner := seedUserForPushSubscriptions(t, db, "user_1", "a@example.com")
	other := seedUserForPushSubscriptions(t, db, "user_2", "b@example.com")

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: owner, Endpoint: "https://a", P256dh: "k1", Auth: "a1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.DeletePushSubscription(ctx, "push_1", other); !errors.Is(err, ErrPushSubscriptionNotFound) {
		t.Errorf("DeletePushSubscription() by another user error = %v, want ErrPushSubscriptionNotFound", err)
	}
	if err := db.DeletePushSubscription(ctx, "push_1", owner); err != nil {
		t.Errorf("DeletePushSubscription() by the owner error = %v, want nil", err)
	}
}

func TestDeletePushSubscriptionByID_RemovesRegardlessOfOwner(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	userID := seedUserForPushSubscriptions(t, db, "user_1", "a@example.com")

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: userID, Endpoint: "https://a", P256dh: "k1", Auth: "a1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.DeletePushSubscriptionByID(ctx, "push_1"); err != nil {
		t.Fatalf("DeletePushSubscriptionByID() error = %v", err)
	}
	rows, err := db.ListPushSubscriptionsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListPushSubscriptionsForUser() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows after delete, want 0", len(rows))
	}
}
