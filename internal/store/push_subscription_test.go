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
	if err := db.SavePushSubscription(ctx, s, 0); err != nil {
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
	}, 0); err != nil {
		t.Fatalf("SavePushSubscription() error = %v", err)
	}
	if err := db.SavePushSubscription(ctx, PushSubscription{
		ID: "push_2", UserID: userID, Endpoint: endpoint, P256dh: "new-key", Auth: "new-auth", CreatedAt: time.Now().UTC(),
	}, 0); err != nil {
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

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: user1, Endpoint: "https://a", P256dh: "k1", Auth: "a1", CreatedAt: time.Now().UTC()}, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_2", UserID: user2, Endpoint: "https://b", P256dh: "k2", Auth: "a2", CreatedAt: time.Now().UTC()}, 0); err != nil {
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

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: owner, Endpoint: "https://a", P256dh: "k1", Auth: "a1", CreatedAt: time.Now().UTC()}, 0); err != nil {
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

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: userID, Endpoint: "https://a", P256dh: "k1", Auth: "a1", CreatedAt: time.Now().UTC()}, 0); err != nil {
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

func TestSavePushSubscription_RefusesAnotherUsersEndpoint(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	a := seedUserForPushSubscriptions(t, db, "user_a", "a@example.com")
	b := seedUserForPushSubscriptions(t, db, "user_b", "b@example.com")

	if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_1", UserID: a, Endpoint: "https://e", P256dh: "ka", Auth: "aa", CreatedAt: time.Now().UTC()}, 0); err != nil {
		t.Fatal(err)
	}
	err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_2", UserID: b, Endpoint: "https://e", P256dh: "kb", Auth: "ab", CreatedAt: time.Now().UTC()}, 0)
	if !errors.Is(err, ErrPushSubscriptionOwned) {
		t.Fatalf("takeover error = %v, want ErrPushSubscriptionOwned", err)
	}
	rows, _ := db.ListPushSubscriptionsForUser(ctx, a)
	if len(rows) != 1 || rows[0].P256dh != "ka" {
		t.Errorf("owner row changed: %+v", rows)
	}
}

func TestSavePushSubscription_PerUserCap(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u := seedUserForPushSubscriptions(t, db, "user_a", "a@example.com")
	save := func(id, endpoint string) error {
		return db.SavePushSubscription(ctx, PushSubscription{ID: id, UserID: u, Endpoint: endpoint, P256dh: "k", Auth: "a", CreatedAt: time.Now().UTC()}, 2)
	}
	if err := save("push_1", "https://e1"); err != nil {
		t.Fatal(err)
	}
	if err := save("push_2", "https://e2"); err != nil {
		t.Fatal(err)
	}
	if err := save("push_3", "https://e3"); !errors.Is(err, ErrPushSubscriptionLimit) {
		t.Errorf("third subscription error = %v, want ErrPushSubscriptionLimit", err)
	}
	if err := save("push_4", "https://e1"); err != nil {
		t.Errorf("re-subscribing an existing endpoint at the cap = %v, want ok", err)
	}
}

func TestRemoveTeamMember_DefaultTeamDropsPushSubscriptions(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveOrganization(ctx, Organization{ID: "org_t", Name: "t", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	seedTeamUser(t, db, "o1", "org_t", TeamRoleOwner, TeamRoleAbilities(TeamRoleOwner))
	seedTeamUser(t, db, "v1", "", "", []string{"read"})
	seedTeamUser(t, db, "v2", "", "", []string{"read"})
	for _, id := range []string{"v1", "v2"} {
		if err := db.SavePushSubscription(ctx, PushSubscription{ID: "push_" + id, UserID: id, Endpoint: "https://" + id, P256dh: "k", Auth: "a", CreatedAt: time.Now().UTC()}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddTeamMember(ctx, "org_t", "v1", TeamRoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveTeamMember(ctx, "org_t", "v1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.ListPushSubscriptionsForUser(ctx, "v1"); len(rows) != 1 {
		t.Errorf("leaving another team removed subscriptions: %d left, want 1", len(rows))
	}
	if err := db.RemoveTeamMember(ctx, DefaultTeamID, "v1"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.ListPushSubscriptionsForUser(ctx, "v1"); len(rows) != 0 {
		t.Errorf("v1 kept %d subscriptions after leaving the default team", len(rows))
	}
	if rows, _ := db.ListPushSubscriptionsForUser(ctx, "v2"); len(rows) != 1 {
		t.Errorf("v2 subscriptions = %d, want untouched", len(rows))
	}
}
