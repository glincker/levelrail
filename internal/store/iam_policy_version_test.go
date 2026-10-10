package store

import (
	"context"
	"testing"
)

func TestPolicyVersions_NumberedAndNewestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SavePolicy(ctx, testPolicy("pol_v", "versioned")); err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{"one", "two", "three"} {
		if err := db.SavePolicyVersion(ctx, "pol_v", "versioned", "", doc, "user:u1"); err != nil {
			t.Fatalf("SavePolicyVersion() error = %v", err)
		}
	}
	got, err := db.ListPolicyVersions(ctx, "pol_v")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Version != 3 || got[0].Document != "three" || got[2].Version != 1 || got[0].Actor != "user:u1" {
		t.Errorf("versions = %+v", got)
	}
}

func TestPolicyVersions_CascadeOnDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SavePolicy(ctx, testPolicy("pol_c", "cascade")); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePolicyVersion(ctx, "pol_c", "cascade", "", "{}", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.DeletePolicy(ctx, "pol_c"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ListPolicyVersions(ctx, "pol_c"); len(got) != 0 {
		t.Errorf("versions after delete = %d, want 0", len(got))
	}
}
