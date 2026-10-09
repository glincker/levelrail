package store

import (
	"context"
	"errors"
	"testing"
)

func TestDatabaseQueryHistory_TrimsPerOwner(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, err := db.AddDatabaseQueryHistory(ctx, DatabaseQueryHistory{DatabaseName: "main", PrincipalType: "user", PrincipalID: "u1", Mode: "read", SQL: "select 1", OK: true}, 3)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.AddDatabaseQueryHistory(ctx, DatabaseQueryHistory{DatabaseName: "main", PrincipalType: "user", PrincipalID: "u2", Mode: "read", SQL: "select 2"}, 3); err != nil {
		t.Fatal(err)
	}
	u1, err := db.ListDatabaseQueryHistory(ctx, "main", "user", "u1", 50)
	if err != nil || len(u1) != 3 {
		t.Fatalf("u1 history = %d, err = %v, want 3", len(u1), err)
	}
	u2, _ := db.ListDatabaseQueryHistory(ctx, "main", "user", "u2", 50)
	if len(u2) != 1 || u2[0].OK {
		t.Fatalf("u2 history = %+v", u2)
	}
	if err := db.ClearDatabaseQueryHistory(ctx, "main", "user", "u1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ListDatabaseQueryHistory(ctx, "main", "user", "u1", 50); len(got) != 0 {
		t.Fatalf("history after clear = %d", len(got))
	}
}

func TestDatabaseSavedQueries(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	q, err := db.SaveDatabaseQuery(ctx, DatabaseSavedQuery{DatabaseName: "main", PrincipalType: "user", PrincipalID: "u1", Name: "users", SQL: "select * from users"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveDatabaseQuery(ctx, DatabaseSavedQuery{DatabaseName: "main", PrincipalType: "user", PrincipalID: "u1", Name: "users", SQL: "select 1"}); !errors.Is(err, ErrSavedQueryExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	if err := db.DeleteDatabaseSavedQuery(ctx, "main", "user", "u2", q.ID); !errors.Is(err, ErrSavedQueryNotFound) {
		t.Fatalf("other owner delete err = %v", err)
	}
	list, _ := db.ListDatabaseSavedQueries(ctx, "main", "user", "u1")
	if len(list) != 1 || list[0].SQL != "select * from users" {
		t.Fatalf("list = %+v", list)
	}
	if err := db.DeleteDatabaseSavedQuery(ctx, "main", "user", "u1", q.ID); err != nil {
		t.Fatal(err)
	}
}
