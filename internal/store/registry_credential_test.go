package store

import (
	"context"
	"errors"
	"testing"
)

func newTestRegistryCredential() RegistryCredential {
	return RegistryCredential{
		ID:           "regcred_test1",
		Name:         "ghcr-bot",
		RegistryHost: "ghcr.io",
		Username:     "bot",
		CreatedAt:    "2026-08-17T00:00:00Z",
	}
}

func TestSaveAndGetRegistryCredential(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestRegistryCredential()
	if err := db.SaveRegistryCredential(ctx, want); err != nil {
		t.Fatalf("SaveRegistryCredential() error = %v", err)
	}

	got, err := db.GetRegistryCredential(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetRegistryCredential() error = %v", err)
	}
	if got != want {
		t.Errorf("GetRegistryCredential() = %+v, want %+v", got, want)
	}
}

func TestGetRegistryCredential_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetRegistryCredential(ctx, "regcred_missing")
	if !errors.Is(err, ErrRegistryCredentialNotFound) {
		t.Fatalf("GetRegistryCredential() error = %v, want ErrRegistryCredentialNotFound", err)
	}
}

func TestGetRegistryCredentialByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestRegistryCredential()
	if err := db.SaveRegistryCredential(ctx, want); err != nil {
		t.Fatalf("SaveRegistryCredential() error = %v", err)
	}

	got, err := db.GetRegistryCredentialByName(ctx, want.Name)
	if err != nil {
		t.Fatalf("GetRegistryCredentialByName() error = %v", err)
	}
	if got != want {
		t.Errorf("GetRegistryCredentialByName() = %+v, want %+v", got, want)
	}
}

func TestGetRegistryCredentialByName_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetRegistryCredentialByName(ctx, "does-not-exist")
	if !errors.Is(err, ErrRegistryCredentialNotFound) {
		t.Fatalf("GetRegistryCredentialByName() error = %v, want ErrRegistryCredentialNotFound", err)
	}
}

func TestListRegistryCredentials_OrderedByCreation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestRegistryCredential()
	first.ID, first.Name, first.CreatedAt = "regcred_a", "cred-a", "2026-08-17T00:00:00Z"
	second := newTestRegistryCredential()
	second.ID, second.Name, second.CreatedAt = "regcred_b", "cred-b", "2026-08-17T00:00:01Z"

	if err := db.SaveRegistryCredential(ctx, second); err != nil {
		t.Fatalf("SaveRegistryCredential(second) error = %v", err)
	}
	if err := db.SaveRegistryCredential(ctx, first); err != nil {
		t.Fatalf("SaveRegistryCredential(first) error = %v", err)
	}

	got, err := db.ListRegistryCredentials(ctx)
	if err != nil {
		t.Fatalf("ListRegistryCredentials() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "regcred_a" || got[1].ID != "regcred_b" {
		t.Fatalf("ListRegistryCredentials() = %+v, want [regcred_a, regcred_b] in creation order", got)
	}
}

func TestUpdateRegistryCredential(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	cred := newTestRegistryCredential()
	if err := db.SaveRegistryCredential(ctx, cred); err != nil {
		t.Fatalf("SaveRegistryCredential() error = %v", err)
	}

	if err := db.UpdateRegistryCredential(ctx, cred.ID, "renamed", "registry.example.com", "new-user"); err != nil {
		t.Fatalf("UpdateRegistryCredential() error = %v", err)
	}

	got, err := db.GetRegistryCredential(ctx, cred.ID)
	if err != nil {
		t.Fatalf("GetRegistryCredential() error = %v", err)
	}
	want := RegistryCredential{ID: cred.ID, Name: "renamed", RegistryHost: "registry.example.com", Username: "new-user", CreatedAt: cred.CreatedAt}
	if got != want {
		t.Errorf("GetRegistryCredential() after update = %+v, want %+v", got, want)
	}
}

func TestUpdateRegistryCredential_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.UpdateRegistryCredential(ctx, "regcred_missing", "x", "registry.example.com", "user")
	if !errors.Is(err, ErrRegistryCredentialNotFound) {
		t.Fatalf("UpdateRegistryCredential() error = %v, want ErrRegistryCredentialNotFound", err)
	}
}

func TestDeleteRegistryCredential(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestRegistryCredential()
	if err := db.SaveRegistryCredential(ctx, want); err != nil {
		t.Fatalf("SaveRegistryCredential() error = %v", err)
	}
	if err := db.DeleteRegistryCredential(ctx, want.ID); err != nil {
		t.Fatalf("DeleteRegistryCredential() error = %v", err)
	}

	_, err := db.GetRegistryCredential(ctx, want.ID)
	if !errors.Is(err, ErrRegistryCredentialNotFound) {
		t.Fatalf("GetRegistryCredential() after delete error = %v, want ErrRegistryCredentialNotFound", err)
	}
}

func TestDeleteRegistryCredential_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteRegistryCredential(ctx, "regcred_missing")
	if !errors.Is(err, ErrRegistryCredentialNotFound) {
		t.Fatalf("DeleteRegistryCredential() error = %v, want ErrRegistryCredentialNotFound", err)
	}
}

func TestSaveRegistryCredential_DuplicateName_Rejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	first := newTestRegistryCredential()
	if err := db.SaveRegistryCredential(ctx, first); err != nil {
		t.Fatalf("SaveRegistryCredential(first) error = %v", err)
	}

	second := newTestRegistryCredential()
	second.ID = "regcred_other"
	if err := db.SaveRegistryCredential(ctx, second); err == nil {
		t.Fatal("SaveRegistryCredential() error = nil, want a UNIQUE constraint error for a duplicate name")
	}
}

func TestDesiredService_RegistryCredentialID_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	svc := DesiredService{Name: "web", Image: "ghcr.io/org/app:v1", Port: 3000, RegistryCredentialID: "regcred_abc"}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.RegistryCredentialID != "regcred_abc" {
		t.Errorf("RegistryCredentialID = %q, want %q", got.RegistryCredentialID, "regcred_abc")
	}
}
