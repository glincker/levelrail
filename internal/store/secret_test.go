package store

import (
	"context"
	"errors"
	"testing"
)

func TestSaveAndGetServiceDEK(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	wrapped := []byte("fake-wrapped-dek-bytes")
	if err := db.SaveServiceDEK(ctx, "web", wrapped); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}

	got, err := db.GetServiceDEK(ctx, "web")
	if err != nil {
		t.Fatalf("GetServiceDEK() error = %v", err)
	}
	if string(got) != string(wrapped) {
		t.Errorf("got %q, want %q", got, wrapped)
	}
}

func TestGetServiceDEK_NotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetServiceDEK(context.Background(), "ghost"); !errors.Is(err, ErrServiceDEKNotFound) {
		t.Errorf("error = %v, want ErrServiceDEKNotFound", err)
	}
}

func TestSaveAndGetSecretValue(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	ciphertext := []byte("fake-ciphertext-bytes")
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", ciphertext); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	got, err := db.GetSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(got) != string(ciphertext) {
		t.Errorf("got %q, want %q", got, ciphertext)
	}
}

func TestSaveSecretValue_UpsertReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}

	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("old")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("new")); err != nil {
		t.Fatalf("SaveSecretValue() (rotate) error = %v", err)
	}

	got, err := db.GetSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("GetSecretValue() error = %v", err)
	}
	if string(got) != "new" {
		t.Errorf("got %q, want %q (rotation must replace, not accumulate)", got, "new")
	}
}

func TestGetSecretValue_NotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetSecretValue(context.Background(), "web", "MISSING"); !errors.Is(err, ErrSecretValueNotFound) {
		t.Errorf("error = %v, want ErrSecretValueNotFound", err)
	}
}

func TestHasSecretValue(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	exists, err := db.HasSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("HasSecretValue() error = %v", err)
	}
	if exists {
		t.Error("HasSecretValue() = true before any value saved, want false")
	}

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek")); err != nil {
		t.Fatalf("SaveServiceDEK() error = %v", err)
	}
	if err := db.SaveSecretValue(ctx, "web", "API_KEY", []byte("ciphertext")); err != nil {
		t.Fatalf("SaveSecretValue() error = %v", err)
	}

	exists, err = db.HasSecretValue(ctx, "web", "API_KEY")
	if err != nil {
		t.Fatalf("HasSecretValue() error = %v", err)
	}
	if !exists {
		t.Error("HasSecretValue() = false after saving a value, want true")
	}
}

func TestSaveServiceDEK_TwoServicesGetIndependentDEKs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveServiceDEK(ctx, "web", []byte("dek-web")); err != nil {
		t.Fatalf("SaveServiceDEK(web) error = %v", err)
	}
	if err := db.SaveServiceDEK(ctx, "worker", []byte("dek-worker")); err != nil {
		t.Fatalf("SaveServiceDEK(worker) error = %v", err)
	}

	webDEK, err := db.GetServiceDEK(ctx, "web")
	if err != nil {
		t.Fatalf("GetServiceDEK(web) error = %v", err)
	}
	workerDEK, err := db.GetServiceDEK(ctx, "worker")
	if err != nil {
		t.Fatalf("GetServiceDEK(worker) error = %v", err)
	}
	if string(webDEK) == string(workerDEK) {
		t.Error("two different services got the same DEK bytes, want independent per-app keys")
	}
}
