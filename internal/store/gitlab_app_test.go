package store

import (
	"context"
	"errors"
	"testing"
)

func newTestGitLabAppConnection() GitLabAppConnection {
	return GitLabAppConnection{
		InstanceURL: "https://gitlab.example.com",
		ClientID:    "test-client-id",
		CreatedAt:   "2026-08-16T00:00:00Z",
	}
}

func TestSaveAndGetGitLabAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestGitLabAppConnection()
	if err := db.SaveGitLabAppConnection(ctx, want); err != nil {
		t.Fatalf("SaveGitLabAppConnection() error = %v", err)
	}

	got, err := db.GetGitLabAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGitLabAppConnection() error = %v", err)
	}
	if got != want {
		t.Errorf("GetGitLabAppConnection() = %+v, want %+v", got, want)
	}
}

func TestGetGitLabAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetGitLabAppConnection(ctx)
	if !errors.Is(err, ErrGitLabAppConnectionNotFound) {
		t.Fatalf("GetGitLabAppConnection() error = %v, want ErrGitLabAppConnectionNotFound", err)
	}
}

func TestSaveGitLabAppConnection_ReplacesExisting(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestGitLabAppConnection()
	if err := db.SaveGitLabAppConnection(ctx, first); err != nil {
		t.Fatalf("SaveGitLabAppConnection(first) error = %v", err)
	}

	second := newTestGitLabAppConnection()
	second.InstanceURL = "https://gitlab.internal.example.com"
	second.ClientID = "second-client-id"
	if err := db.SaveGitLabAppConnection(ctx, second); err != nil {
		t.Fatalf("SaveGitLabAppConnection(second) error = %v", err)
	}

	got, err := db.GetGitLabAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetGitLabAppConnection() error = %v", err)
	}
	if got != second {
		t.Errorf("GetGitLabAppConnection() = %+v, want the second (replacing) connection %+v", got, second)
	}
}

func TestDeleteGitLabAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveGitLabAppConnection(ctx, newTestGitLabAppConnection()); err != nil {
		t.Fatalf("SaveGitLabAppConnection() error = %v", err)
	}
	if err := db.DeleteGitLabAppConnection(ctx); err != nil {
		t.Fatalf("DeleteGitLabAppConnection() error = %v", err)
	}

	_, err := db.GetGitLabAppConnection(ctx)
	if !errors.Is(err, ErrGitLabAppConnectionNotFound) {
		t.Fatalf("GetGitLabAppConnection() after delete error = %v, want ErrGitLabAppConnectionNotFound", err)
	}
}

func TestDeleteGitLabAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteGitLabAppConnection(ctx)
	if !errors.Is(err, ErrGitLabAppConnectionNotFound) {
		t.Fatalf("DeleteGitLabAppConnection() error = %v, want ErrGitLabAppConnectionNotFound", err)
	}
}

func TestGitLabAppSecretsKey_CannotCollideWithServiceName(t *testing.T) {
	key := GitLabAppSecretsKey()
	if key == "" {
		t.Fatal("GitLabAppSecretsKey() returned empty string")
	}
	if !containsSlash(key) {
		t.Fatalf("GitLabAppSecretsKey() = %q, want a %q-containing sentinel key", key, "/")
	}
}
