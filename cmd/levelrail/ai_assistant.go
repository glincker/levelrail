package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// setupAIAssistantEngine builds the assistant engine. Each chat turn's tools
// call this instance's own REST API at dialAddr (loopback) with a token the
// API minted for that turn and its sender only; nothing is persisted.
func setupAIAssistantEngine(ctx context.Context, db *store.DB, secretsManager *secrets.Manager, dialAddr, brandName string, logger *slog.Logger) (*ai.Engine, error) {
	if err := retireAIAssistantRootToken(ctx, db, secretsManager, logger); err != nil {
		return nil, err
	}
	engine := ai.NewEngine(db, secretsManager, nil, nil, brandName)
	engine.SetProviders(ai.NewVault(db, secretsManager, logger))
	engine.SetUsage(db)
	engine.SetLogger(logger)
	engine.SetToolFactory(func(ctx context.Context, token string) (ai.ToolExecutor, error) {
		client := apiclient.NewClient("http://"+dialAddr, token) // NOSONAR: dialAddr is always loopback, this control plane calling its own local REST API in-process, never a real network HTTPS gap
		return ai.NewToolCaller(ctx, client)
	})
	return engine, nil
}

// retireAIAssistantRootToken removes the persistent root token older
// releases stored for the assistant.
func retireAIAssistantRootToken(ctx context.Context, db *store.DB, secretsManager *secrets.Manager, logger *slog.Logger) error {
	key := store.AIInternalTokenSecretsKey()
	exists, err := secretsManager.Exists(ctx, key, "token")
	if err != nil {
		return fmt.Errorf("check legacy ai assistant token: %w", err)
	}
	if exists {
		if err := secretsManager.DeleteAll(ctx, key); err != nil {
			return fmt.Errorf("delete legacy ai assistant token: %w", err)
		}
	}
	n, err := db.RevokeSystemTokensByName(ctx, api.AIAssistantTokenName)
	if err != nil {
		return fmt.Errorf("revoke legacy ai assistant token: %w", err)
	}
	if exists || n > 0 {
		logger.Info("ai assistant: retired the persistent root token, turns now use per-sender tokens", slog.Int("revoked", n))
	}
	return nil
}
