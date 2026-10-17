package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// setupAIAssistantEngine builds the assistant engine. Each chat turn's tools
// call this instance's own REST API at dialAddr (loopback) with a token the
// API minted for that turn and its sender only; nothing is persisted.
func setupAIAssistantEngine(ctx context.Context, db *store.DB, secretsManager *secrets.Manager, dialAddr, brandName string, logger *slog.Logger) (*ai.Engine, error) {
	engine := ai.NewEngine(db, ai.NewVault(db, secretsManager, logger), nil, brandName)
	engine.SetUsage(db)
	engine.SetLogger(logger)
	engine.SetToolFactory(func(ctx context.Context, token string) (ai.ToolExecutor, error) {
		client := apiclient.NewClient("http://"+dialAddr, token) // NOSONAR: dialAddr is always loopback, this control plane calling its own local REST API in-process, never a real network HTTPS gap
		return ai.NewToolCaller(ctx, client)
	})
	return engine, nil
}
