package store

import (
	"context"
	"testing"
)

func TestDeployURLCopiesModeRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if got, err := db.GetDeployURLCopiesMode(ctx); err != nil || got != "" {
		t.Fatalf("fresh = %q, %v, want unset", got, err)
	}
	if err := db.SetDeployURLCopiesMode(ctx, "public"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetDeployURLCopiesMode(ctx); got != "public" {
		t.Fatalf("got %q, want public", got)
	}
	if err := db.SetDeployURLCopiesMode(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.GetDeployURLCopiesMode(ctx); got != "" {
		t.Fatalf("got %q after clear, want unset", got)
	}
}
