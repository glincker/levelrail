package store

import (
	"context"
	"errors"
	"testing"
)

func newTestGiteaAppConnection() GiteaAppConnection {
	return GiteaAppConnection{
		InstanceURL: "https://git.example.com",
		ClientID:    "test-client-id",
		CreatedAt:   "2026-08-20T00:00:00Z",
	}
}

func TestSaveAndGetGiteaAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestGiteaAppConnection()
	if err := db.SaveGiteaAppConnection(ctx, want); err != nil {
		t.Fatalf("SaveGiteaAppConnection() error = %v", err)
	}

	got, err := db.GetGiteaAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGiteaAppConnection() error = %v", err)
	}
	if got != want {
		t.Errorf("GetGiteaAppConnection() = %+v, want %+v", got, want)
	}
}

func TestGetGiteaAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetGiteaAppConnection(ctx)
	if !errors.Is(err, ErrGiteaAppConnectionNotFound) {
		t.Fatalf("GetGiteaAppConnection() error = %v, want ErrGiteaAppConnectionNotFound", err)
	}
}

func TestSaveGiteaAppConnection_ReplacesExisting(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestGiteaAppConnection()
	if err := db.SaveGiteaAppConnection(ctx, first); err != nil {
		t.Fatalf("SaveGiteaAppConnection(first) error = %v", err)
	}

	second := newTestGiteaAppConnection()
	second.ClientID = "second-client-id"
	if err := db.SaveGiteaAppConnection(ctx, second); err != nil {
		t.Fatalf("SaveGiteaAppConnection(second) error = %v", err)
	}

	got, err := db.GetGiteaAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGiteaAppConnection() error = %v", err)
	}
	if got != second {
		t.Errorf("GetGiteaAppConnection() = %+v, want the second (replacing) connection %+v", got, second)
	}
}

func TestDeleteGiteaAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveGiteaAppConnection(ctx, newTestGiteaAppConnection()); err != nil {
		t.Fatalf("SaveGiteaAppConnection() error = %v", err)
	}
	if err := db.DeleteGiteaAppConnection(ctx); err != nil {
		t.Fatalf("DeleteGiteaAppConnection() error = %v", err)
	}

	_, err := db.GetGiteaAppConnection(ctx)
	if !errors.Is(err, ErrGiteaAppConnectionNotFound) {
		t.Fatalf("GetGiteaAppConnection() after delete error = %v, want ErrGiteaAppConnectionNotFound", err)
	}
}

func TestDeleteGiteaAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteGiteaAppConnection(ctx)
	if !errors.Is(err, ErrGiteaAppConnectionNotFound) {
		t.Fatalf("DeleteGiteaAppConnection() error = %v, want ErrGiteaAppConnectionNotFound", err)
	}
}

func TestGiteaAppSecretsKey_CannotCollideWithServiceName(t *testing.T) {
	key := GiteaAppSecretsKey()
	if key == "" {
		t.Fatal("GiteaAppSecretsKey() returned empty string")
	}
	if !containsSlash(key) {
		t.Fatalf("GiteaAppSecretsKey() = %q, want a %q-containing sentinel key", key, "/")
	}
}
