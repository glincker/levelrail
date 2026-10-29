package store

import (
	"context"
	"errors"
	"testing"
)

func TestAppTypeVersionsAreImmutable(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveAppTypeDraft(ctx, "api", `{"name":"api"}`, "root"); err != nil {
		t.Fatal(err)
	}
	v := AppTypeVersion{Name: "api", Version: 1, Document: `{"name":"api","version":1}`, Digest: "d1", PublishedBy: "root"}
	if err := db.InsertAppTypeVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertAppTypeVersion(ctx, v); !errors.Is(err, ErrAppTypeVersionExists) {
		t.Fatalf("republish = %v, want ErrAppTypeVersionExists", err)
	}
	got, err := db.GetAppTypeVersion(ctx, "api", 0)
	if err != nil || got.Version != 1 || got.Digest != "d1" {
		t.Fatalf("latest = %+v, %v", got, err)
	}
	list, _ := db.ListAppTypes(ctx)
	if len(list) != 1 || list[0].LatestVersion != 1 {
		t.Fatalf("list = %+v", list)
	}
}
