package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSetupAIAssistantEngine_RetiresPersistentRootToken(t *testing.T) {
	ctx := context.Background()
	db := openCredentialsTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	sm := secrets.NewManager(db, mk)
	legacy, rec, err := api.MintAPIToken(ctx, db, api.AIAssistantTokenName, []string{api.AbilityRoot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.SetValue(ctx, store.AIInternalTokenSecretsKey(), "token", legacy); err != nil {
		t.Fatal(err)
	}

	if _, err := setupAIAssistantEngine(ctx, db, sm, "127.0.0.1:1", "", slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("setupAIAssistantEngine() error = %v", err)
	}

	if exists, err := sm.Exists(ctx, store.AIInternalTokenSecretsKey(), "token"); err != nil || exists {
		t.Errorf("persistent assistant token still in the secrets store (exists=%v, err=%v)", exists, err)
	}
	got, err := db.GetAPITokenByID(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RevokedAt == nil {
		t.Error("persistent root assistant token still live, want revoked at boot")
	}
}
