package store

import (
	"context"
	"errors"
	"testing"
)

func TestRenameTag(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedTagService(t, db, "web")
	a, b := newTestTag("tag_1", "production"), newTestTag("tag_2", "staging")
	for _, tag := range []Tag{a, b} {
		if err := db.SaveTag(ctx, tag); err != nil {
			t.Fatalf("SaveTag() error = %v", err)
		}
	}
	if err := db.AttachAppTag(ctx, a.ID, "web"); err != nil {
		t.Fatalf("AttachAppTag() error = %v", err)
	}

	got, err := db.RenameTag(ctx, a.ID, "  Prod ")
	if err != nil || got.Name != "prod" {
		t.Fatalf("RenameTag() = %+v, %v, want name prod", got, err)
	}
	if _, err := db.RenameTag(ctx, a.ID, "staging"); !errors.Is(err, ErrTagNameTaken) {
		t.Errorf("RenameTag() to taken name error = %v, want ErrTagNameTaken", err)
	}
	if _, err := db.RenameTag(ctx, "tag_missing", "other"); !errors.Is(err, ErrTagNotFound) {
		t.Errorf("RenameTag() unknown id error = %v, want ErrTagNotFound", err)
	}

	usage, err := db.CountTagUsage(ctx)
	if err != nil || usage[a.ID].Apps != 1 || usage[b.ID].Apps != 0 {
		t.Errorf("CountTagUsage() = %+v, %v, want 1 app on the renamed tag", usage, err)
	}
}
