package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCloudConnectionsCRUDAndCheck(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	//nolint:gosec // fixture reference, not a credential
	c := CloudConnection{Name: "oci-test", Provider: "oracle", AuthMethod: "secret_ref", Config: `{"region":"us-ashburn-1"}`, CredentialRef: "secret://v/a#k", CreatedBy: "u1"}
	if err := db.CreateCloudConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCloudConnection(ctx, c); !errors.Is(err, ErrCloudConnectionExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	if err := db.SetCloudConnectionCheck(ctx, "oci-test", "ok", time.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetCloudConnection(ctx, "oci-test")
	if err != nil || got.LastCheckClass != "ok" || got.LastCheckedAt.IsZero() || got.CreatedBy != "u1" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	c.TeamID = "team1"
	if err := db.UpdateCloudConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetCloudConnection(ctx, "oci-test")
	if got.TeamID != "team1" || got.LastCheckClass != "" {
		t.Fatalf("update should clear the stale check: %+v", got)
	}
	if err := db.DeleteCloudConnection(ctx, "oci-test"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteCloudConnection(ctx, "oci-test"); !errors.Is(err, ErrCloudConnectionNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}
