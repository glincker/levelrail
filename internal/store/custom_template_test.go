package store

import (
	"context"
	"errors"
	"testing"
)

func newTestCustomTemplate() CustomTemplate {
	return CustomTemplate{
		ID:          "custom-abc123",
		Name:        "My stack",
		Description: "web + redis, saved from production",
		Compose:     "version: \"3.8\"\nservices:\n  web:\n    image: myorg/web:1.0.0\n",
		SourceApp:   "production",
		CreatedBy:   "user_1",
		CreatedAt:   "2026-10-01T00:00:00Z",
		UpdatedAt:   "2026-10-01T00:00:00Z",
	}
}

func TestSaveAndGetCustomTemplate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestCustomTemplate()
	if err := db.SaveCustomTemplate(ctx, want); err != nil {
		t.Fatalf("SaveCustomTemplate() error = %v", err)
	}

	got, err := db.GetCustomTemplate(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetCustomTemplate() error = %v", err)
	}
	if got != want {
		t.Errorf("GetCustomTemplate() = %+v, want %+v", got, want)
	}
}

func TestGetCustomTemplate_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetCustomTemplate(ctx, "custom-missing")
	if !errors.Is(err, ErrCustomTemplateNotFound) {
		t.Errorf("GetCustomTemplate() error = %v, want ErrCustomTemplateNotFound", err)
	}
}

func TestListCustomTemplates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if got, err := db.ListCustomTemplates(ctx); err != nil || len(got) != 0 {
		t.Fatalf("ListCustomTemplates() on empty store = %v, %v, want empty slice, nil error", got, err)
	}

	first := newTestCustomTemplate()
	second := newTestCustomTemplate()
	second.ID, second.Name, second.CreatedAt = "custom-def456", "Another stack", "2026-10-01T00:00:01Z"

	if err := db.SaveCustomTemplate(ctx, first); err != nil {
		t.Fatalf("SaveCustomTemplate(first) error = %v", err)
	}
	if err := db.SaveCustomTemplate(ctx, second); err != nil {
		t.Fatalf("SaveCustomTemplate(second) error = %v", err)
	}

	got, err := db.ListCustomTemplates(ctx)
	if err != nil {
		t.Fatalf("ListCustomTemplates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListCustomTemplates() returned %d templates, want 2", len(got))
	}
	if got[0].ID != first.ID || got[1].ID != second.ID {
		t.Errorf("ListCustomTemplates() order = [%s, %s], want oldest-first [%s, %s]", got[0].ID, got[1].ID, first.ID, second.ID)
	}
}

func TestDeleteCustomTemplate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestCustomTemplate()
	if err := db.SaveCustomTemplate(ctx, want); err != nil {
		t.Fatalf("SaveCustomTemplate() error = %v", err)
	}

	if err := db.DeleteCustomTemplate(ctx, want.ID); err != nil {
		t.Fatalf("DeleteCustomTemplate() error = %v", err)
	}

	if _, err := db.GetCustomTemplate(ctx, want.ID); !errors.Is(err, ErrCustomTemplateNotFound) {
		t.Errorf("GetCustomTemplate() after delete error = %v, want ErrCustomTemplateNotFound", err)
	}
}

func TestDeleteCustomTemplate_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.DeleteCustomTemplate(ctx, "custom-missing"); !errors.Is(err, ErrCustomTemplateNotFound) {
		t.Errorf("DeleteCustomTemplate() error = %v, want ErrCustomTemplateNotFound", err)
	}
}
