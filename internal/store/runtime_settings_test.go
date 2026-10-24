package store

import (
	"testing"
	"time"
)

func TestRuntimeSettings_SetListDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	now := time.Now()

	if rows, err := db.ListRuntimeSettings(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("empty list = %v, %v", rows, err)
	}
	if err := db.SetRuntimeSetting(ctx, "a.b", "true", "alice", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRuntimeSetting(ctx, "a.b", "false", "bob", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SetRuntimeSetting(ctx, "a.c", "5m", "", now); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListRuntimeSettings(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("list = %v, %v", rows, err)
	}
	if rows[0].Key != "a.b" || rows[0].Value != "false" || rows[0].Actor != "bob" || rows[0].UpdatedAt.IsZero() {
		t.Fatalf("upsert row = %+v", rows[0])
	}
	if err := db.DeleteRuntimeSetting(ctx, "a.b"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRuntimeSetting(ctx, "missing"); err != nil {
		t.Fatalf("deleting a missing key must not fail: %v", err)
	}
	if rows, _ := db.ListRuntimeSettings(ctx); len(rows) != 1 || rows[0].Key != "a.c" {
		t.Fatalf("after delete = %+v", rows)
	}
}
