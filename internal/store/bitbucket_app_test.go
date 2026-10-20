package store

import (
	"context"
	"errors"
	"testing"
)

func newTestBitbucketAppConnection() BitbucketAppConnection {
	return BitbucketAppConnection{
		Key:       "test-key",
		CreatedAt: "2026-08-20T00:00:00Z",
	}
}

func TestSaveAndGetBitbucketAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestBitbucketAppConnection()
	if err := db.SaveBitbucketAppConnection(ctx, want); err != nil {
		t.Fatalf("SaveBitbucketAppConnection() error = %v", err)
	}

	got, err := db.GetBitbucketAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetBitbucketAppConnection() error = %v", err)
	}
	if got != want {
		t.Errorf("GetBitbucketAppConnection() = %+v, want %+v", got, want)
	}
}

func TestGetBitbucketAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetBitbucketAppConnection(ctx)
	if !errors.Is(err, ErrBitbucketAppConnectionNotFound) {
		t.Fatalf("GetBitbucketAppConnection() error = %v, want ErrBitbucketAppConnectionNotFound", err)
	}
}

func TestSaveBitbucketAppConnection_ReplacesExisting(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestBitbucketAppConnection()
	if err := db.SaveBitbucketAppConnection(ctx, first); err != nil {
		t.Fatalf("SaveBitbucketAppConnection(first) error = %v", err)
	}

	second := newTestBitbucketAppConnection()
	second.Key = "second-key"
	if err := db.SaveBitbucketAppConnection(ctx, second); err != nil {
		t.Fatalf("SaveBitbucketAppConnection(second) error = %v", err)
	}

	got, err := db.GetBitbucketAppConnection(ctx)
	if err != nil {
		t.Fatalf("GetBitbucketAppConnection() error = %v", err)
	}
	if got != second {
		t.Errorf("GetBitbucketAppConnection() = %+v, want the second (replacing) connection %+v", got, second)
	}
}

func TestDeleteBitbucketAppConnection(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveBitbucketAppConnection(ctx, newTestBitbucketAppConnection()); err != nil {
		t.Fatalf("SaveBitbucketAppConnection() error = %v", err)
	}
	if err := db.DeleteBitbucketAppConnection(ctx); err != nil {
		t.Fatalf("DeleteBitbucketAppConnection() error = %v", err)
	}

	_, err := db.GetBitbucketAppConnection(ctx)
	if !errors.Is(err, ErrBitbucketAppConnectionNotFound) {
		t.Fatalf("GetBitbucketAppConnection() after delete error = %v, want ErrBitbucketAppConnectionNotFound", err)
	}
}

func TestDeleteBitbucketAppConnection_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteBitbucketAppConnection(ctx)
	if !errors.Is(err, ErrBitbucketAppConnectionNotFound) {
		t.Fatalf("DeleteBitbucketAppConnection() error = %v, want ErrBitbucketAppConnectionNotFound", err)
	}
}

func TestBitbucketAppSecretsKey_CannotCollideWithServiceName(t *testing.T) {
	key := BitbucketAppSecretsKey()
	if key == "" {
		t.Fatal("BitbucketAppSecretsKey() returned empty string")
	}
	if !containsSlash(key) {
		t.Fatalf("BitbucketAppSecretsKey() = %q, want a %q-containing sentinel key", key, "/")
	}
}
