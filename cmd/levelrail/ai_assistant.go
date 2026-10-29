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
func setupAIAssistantEngine(_ context.Context, db *store.DB, secretsManager *secrets.Manager, dialAddr, brandName string, logger *slog.Logger) *ai.Engine {
	engine := ai.NewEngine(db, ai.NewVault(db, secretsManager, logger), nil, brandName)
	engine.SetUsage(db)
	engine.SetLogger(logger)
	engine.SetModeSource(assistantModeSource(db.GetAIControlSettings))
	engine.SetToolFactory(func(ctx context.Context, token string) (ai.ToolExecutor, error) {
		client := apiclient.NewClient("http://"+dialAddr, token) // NOSONAR: dialAddr is always loopback, this control plane calling its own local REST API in-process, never a real network HTTPS gap
		return ai.NewToolCaller(ctx, client)
	})
	return engine
}

// assistantModeSource resolves the AI control mode per tool call. An unreadable or
// unknown mode maps to off so the write guard fails closed.
func assistantModeSource(get func(context.Context) (store.AIControlSettings, error)) ai.ModeSource {
	return func(ctx context.Context) string {
		s, err := get(ctx)
		if err != nil || !store.ValidAIControlMode(s.Mode) {
			return store.AIControlModeOff
		}
		return s.Mode
	}
}
