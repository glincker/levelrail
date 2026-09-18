package store

import (
	"context"
	"errors"
	"testing"
)

func TestSetServicePreviewEnvOverride(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	svc := DesiredService{Name: "web", Image: "img:v1", Port: 8080, Env: map[string]string{"SHARED": "prod-value"}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	shared := "preview-value"
	if err := db.SetServicePreviewEnvOverride(ctx, "web", "SHARED", &shared); err != nil {
		t.Fatalf("SetServicePreviewEnvOverride() error = %v", err)
	}
	other := "preview-only"
	if err := db.SetServicePreviewEnvOverride(ctx, "web", "PREVIEW_ONLY", &other); err != nil {
		t.Fatalf("SetServicePreviewEnvOverride() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if len(got.PreviewEnvOverrides) != 2 {
		t.Fatalf("PreviewEnvOverrides = %+v, want 2 entries", got.PreviewEnvOverrides)
	}
	if got.PreviewEnvOverrides["SHARED"] != "preview-value" {
		t.Errorf("PreviewEnvOverrides[SHARED] = %q, want %q", got.PreviewEnvOverrides["SHARED"], "preview-value")
	}
	// The parent's own inherited env is untouched: an override is only
	// ever applied at preview-creation time, never to the parent itself.
	if got.Env["SHARED"] != "prod-value" {
		t.Errorf("Env[SHARED] = %q, want %q (override must not touch the parent's own env)", got.Env["SHARED"], "prod-value")
	}

	// Removing one key leaves the other untouched: a narrow, single-key
	// mutation, not a full-record replace.
	if err := db.SetServicePreviewEnvOverride(ctx, "web", "SHARED", nil); err != nil {
		t.Fatalf("SetServicePreviewEnvOverride(nil) error = %v", err)
	}
	got, err = db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if len(got.PreviewEnvOverrides) != 1 {
		t.Fatalf("PreviewEnvOverrides = %+v, want 1 entry after removal", got.PreviewEnvOverrides)
	}
	if _, ok := got.PreviewEnvOverrides["PREVIEW_ONLY"]; !ok {
		t.Errorf("PreviewEnvOverrides = %+v, want PREVIEW_ONLY to remain", got.PreviewEnvOverrides)
	}
	if got.Image != "img:v1" || got.Port != 8080 {
		t.Errorf("SetServicePreviewEnvOverride must not touch other fields: got Image=%q Port=%d", got.Image, got.Port)
	}
}

func TestSetServicePreviewEnvOverride_UnknownService(t *testing.T) {
	value := "x"
	db := openTestDB(t)
	err := db.SetServicePreviewEnvOverride(context.Background(), "missing", "KEY", &value)
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("error = %v, want ErrServiceNotFound", err)
	}
}

// TestSaveDesiredService_PreservesPreviewEnvOverrides confirms an
// ordinary app.yaml redeploy (SaveDesiredService's own full-record
// replace) never wipes a preview override: preview_env_overrides is
// deliberately excluded from that method's INSERT/UPDATE columns, the
// same log_drain treatment.
func TestSaveDesiredService_PreservesPreviewEnvOverrides(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	value := "preview-value"
	if err := db.SetServicePreviewEnvOverride(ctx, "web", "SHARED", &value); err != nil {
		t.Fatalf("SetServicePreviewEnvOverride() error = %v", err)
	}

	// A redeploy from app.yaml: same name, new image, no mention of
	// PreviewEnvOverrides at all.
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() redeploy error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Image != "img:v2" {
		t.Fatalf("Image = %q, want %q", got.Image, "img:v2")
	}
	if got.PreviewEnvOverrides["SHARED"] != "preview-value" {
		t.Errorf("PreviewEnvOverrides[SHARED] = %q, want %q to survive an ordinary redeploy", got.PreviewEnvOverrides["SHARED"], "preview-value")
	}
}
