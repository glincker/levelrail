package store

import (
	"context"
	"errors"
	"testing"
)

func TestSecretConnectionsCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	c := SecretConnection{Name: "prod-vault", Type: "vault", Config: `{"address":"https://v:8200"}`, CreatedBy: "u1"}
	if err := db.CreateSecretConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSecretConnection(ctx, c); !errors.Is(err, ErrSecretConnectionExists) {
		t.Fatalf("duplicate err = %v", err)
	}
	c.Config = `{"address":"https://w:8200"}`
	if err := db.UpdateSecretConnection(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSecretConnection(ctx, "prod-vault")
	if err != nil || got.Config != c.Config || got.CreatedBy != "u1" || got.CreatedAt.IsZero() {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if err := db.DeleteSecretConnection(ctx, "prod-vault"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSecretConnection(ctx, "prod-vault"); !errors.Is(err, ErrSecretConnectionNotFound) {
		t.Fatalf("after delete err = %v", err)
	}
}
