package store

import (
	"context"
	"errors"
	"testing"
)

func TestSetServiceBranchEnvOverride(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	svc := DesiredService{Name: "web", Image: "img:v1", Port: 8080, Env: map[string]string{"SHARED": "prod-value"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	id, err := db.SetServiceBranchEnvOverride(ctx, "web", "release/*", "SHARED", "release-value", false)
	if err != nil {
		t.Fatalf("SetServiceBranchEnvOverride() error = %v", err)
	}
	if id == "" {
		t.Fatal("SetServiceBranchEnvOverride() returned empty id")
	}

	overrides, err := db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want 1 entry", overrides)
	}
	if overrides[0].ID != id || overrides[0].BranchPattern != "release/*" || overrides[0].Key != "SHARED" || overrides[0].Value != "release-value" || overrides[0].Secret {
		t.Errorf("overrides[0] = %+v, want id=%q pattern=release/* key=SHARED value=release-value secret=false", overrides[0], id)
	}

	// A second call with the same (service, branch, key) upserts in
	// place: same id, new value, never a duplicate row.
	sameID, err := db.SetServiceBranchEnvOverride(ctx, "web", "release/*", "SHARED", "release-value-2", false)
	if err != nil {
		t.Fatalf("SetServiceBranchEnvOverride() second call error = %v", err)
	}
	if sameID != id {
		t.Errorf("SetServiceBranchEnvOverride() second call id = %q, want %q (upsert must keep the same id)", sameID, id)
	}
	overrides, err = db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 || overrides[0].Value != "release-value-2" {
		t.Fatalf("overrides = %+v, want 1 entry with updated value", overrides)
	}

	// The parent's own env is untouched: a branch override is only ever
	// applied at preview-deploy time, never to the parent itself.
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Env["SHARED"] != "prod-value" {
		t.Errorf("Env[SHARED] = %q, want %q (override must not touch the parent's own env)", got.Env["SHARED"], "prod-value")
	}
}

func TestSetServiceBranchEnvOverride_Secret(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// secret=true: the value column must never carry the plaintext,
	// mirroring SetEnvironmentSecretEnvVar's own "value stays ''" rule.
	id, err := db.SetServiceBranchEnvOverride(ctx, "web", "main", "API_KEY", "should-not-be-stored", true)
	if err != nil {
		t.Fatalf("SetServiceBranchEnvOverride() error = %v", err)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want 1 entry", overrides)
	}
	if !overrides[0].Secret {
		t.Error("overrides[0].Secret = false, want true")
	}
	if overrides[0].Value != "" {
		t.Errorf("overrides[0].Value = %q, want empty (secret plaintext must never be stored here)", overrides[0].Value)
	}
	if overrides[0].ID != id {
		t.Errorf("overrides[0].ID = %q, want %q", overrides[0].ID, id)
	}
}

func TestDeleteServiceBranchEnvOverride(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	id, err := db.SetServiceBranchEnvOverride(ctx, "web", "main", "KEY", "value", false)
	if err != nil {
		t.Fatalf("SetServiceBranchEnvOverride() error = %v", err)
	}

	if err := db.DeleteServiceBranchEnvOverride(ctx, "web", id); err != nil {
		t.Fatalf("DeleteServiceBranchEnvOverride() error = %v", err)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides = %+v, want none after delete", overrides)
	}

	if err := db.DeleteServiceBranchEnvOverride(ctx, "web", id); !errors.Is(err, ErrBranchEnvOverrideNotFound) {
		t.Errorf("DeleteServiceBranchEnvOverride() second call error = %v, want ErrBranchEnvOverrideNotFound", err)
	}
}

func TestDeleteServiceBranchEnvOverride_WrongService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService(web) error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "other", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService(other) error = %v", err)
	}
	id, err := db.SetServiceBranchEnvOverride(ctx, "web", "main", "KEY", "value", false)
	if err != nil {
		t.Fatalf("SetServiceBranchEnvOverride() error = %v", err)
	}

	// "other" must never be able to delete "web"'s own override by id,
	// even if it somehow learns the id: this is the guard IAM's
	// per-app resource scoping (appResourceFromPath) relies on.
	if err := db.DeleteServiceBranchEnvOverride(ctx, "other", id); !errors.Is(err, ErrBranchEnvOverrideNotFound) {
		t.Errorf("DeleteServiceBranchEnvOverride(wrong service) error = %v, want ErrBranchEnvOverrideNotFound", err)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want web's override to survive", overrides)
	}
}

func TestListServiceBranchEnvOverrides_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides = %+v, want none", overrides)
	}
}

func TestListServiceBranchEnvOverrides_MultipleBranchesSameKey(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	// The same key under two different branch patterns is two distinct
	// rows, not a conflict: the unique constraint is on (service,
	// branch_pattern, key), not (service, key).
	if _, err := db.SetServiceBranchEnvOverride(ctx, "web", "release/*", "FEATURE_FLAG", "on", false); err != nil {
		t.Fatalf("SetServiceBranchEnvOverride(release/*) error = %v", err)
	}
	if _, err := db.SetServiceBranchEnvOverride(ctx, "web", "main", "FEATURE_FLAG", "off", false); err != nil {
		t.Fatalf("SetServiceBranchEnvOverride(main) error = %v", err)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(ctx, "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("overrides = %+v, want 2 entries", overrides)
	}
}
