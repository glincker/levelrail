package store

import (
	"context"
	"testing"
)

func TestTemplateCompatLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := db.UpsertTemplateCompat(ctx, "ollama", "arm64", "arch_mismatch", "no build"); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.ListTemplateCompat(ctx)
	if err != nil || len(rows) != 1 || rows[0].Failures != 2 {
		t.Fatalf("want one row with 2 failures, got %+v, %v", rows, err)
	}
	if err := db.ClearTemplateCompat(ctx, "ollama", "arm64"); err != nil {
		t.Fatal(err)
	}
	if rows, _ = db.ListTemplateCompat(ctx); len(rows) != 0 {
		t.Fatalf("want cleared, got %+v", rows)
	}
}
