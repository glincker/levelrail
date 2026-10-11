package store

import (
	"context"
	"errors"
	"testing"
)

func TestSavedLogQuery_CRUDAndVisibility(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	mk := func(id, name, app, owner, vis string, pinned bool) SavedLogQuery {
		return SavedLogQuery{ID: id, Name: name, App: app, SpecJSON: `{"q":"boom"}`, OwnerType: PrincipalTypeUser,
			OwnerID: owner, Visibility: vis, Pinned: pinned}
	}
	for _, q := range []SavedLogQuery{
		mk("a", "errors", "web", "u1", SavedLogQueryPrivate, false),
		mk("b", "slow", "web", "u2", SavedLogQueryInstance, true),
		mk("c", "mine only", "web", "u2", SavedLogQueryPrivate, false),
		mk("d", "other app", "api", "u1", SavedLogQueryPrivate, false),
	} {
		if err := db.CreateSavedLogQuery(ctx, q); err != nil {
			t.Fatalf("create %s: %v", q.ID, err)
		}
	}

	if err := db.CreateSavedLogQuery(ctx, mk("e", "errors", "web", "u1", SavedLogQueryPrivate, false)); !errors.Is(err, ErrSavedLogQueryExists) {
		t.Fatalf("duplicate name = %v, want ErrSavedLogQueryExists", err)
	}

	got, err := db.ListSavedLogQueries(ctx, "u1", "web")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, q := range got {
		ids[q.ID] = true
	}
	if !ids["a"] || !ids["b"] || ids["c"] || ids["d"] {
		t.Fatalf("u1 sees %v, want a and b only (own + instance for web)", ids)
	}
	if got[0].ID != "b" {
		t.Fatalf("pinned must sort first, got %s", got[0].ID)
	}

	q, _ := db.GetSavedLogQuery(ctx, "a")
	q.Name = "errors v2"
	if err := db.UpdateSavedLogQuery(ctx, q); err != nil {
		t.Fatal(err)
	}
	if q2, _ := db.GetSavedLogQuery(ctx, "a"); q2.Name != "errors v2" {
		t.Fatalf("name = %q", q2.Name)
	}
	if err := db.DeleteSavedLogQuery(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSavedLogQuery(ctx, "a"); !errors.Is(err, ErrSavedLogQueryNotFound) {
		t.Fatalf("second delete = %v", err)
	}
}
