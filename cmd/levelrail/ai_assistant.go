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

const aiInternalTokenEnvKey = "token"

// setupAIAssistantEngine mints (or reuses, across restarts) a
// root-scoped API token this control plane holds only in memory and
// inside secretsManager's envelope encryption, never surfaced to an
// operator through any API or UI, so the embedded AI assistant can call
// this instance's own REST API through the exact same apiclient/
// mcptools path an external MCP client already uses (internal/ai's
// ToolCaller). dialAddr is the loopback address this same process is
// about to listen on, the same value api.WithDashboardDial's own caller
// resolves via dashboardDialAddr(httpAddr()).
func setupAIAssistantEngine(ctx context.Context, db *store.DB, secretsManager *secrets.Manager, dialAddr, brandName string, logger *slog.Logger) (*ai.Engine, error) {
	key := store.AIInternalTokenSecretsKey()

	exists, err := secretsManager.Exists(ctx, key, aiInternalTokenEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check ai assistant internal token: %w", err)
	}

	var plaintext string
	if exists {
		plaintext, err = secretsManager.Resolve(ctx, key, aiInternalTokenEnvKey)
		if err != nil {
			return nil, fmt.Errorf("resolve ai assistant internal token: %w", err)
		}
	} else {
		plaintext, _, err = api.MintAPIToken(ctx, db, "AI Assistant (internal)", []string{api.AbilityRoot}, nil)
		if err != nil {
			return nil, fmt.Errorf("mint ai assistant internal token: %w", err)
		}
		if err := secretsManager.SetValue(ctx, key, aiInternalTokenEnvKey, plaintext); err != nil {
			return nil, fmt.Errorf("store ai assistant internal token: %w", err)
		}
		logger.Info("ai assistant: minted internal self-call token")
	}

	client := apiclient.NewClient("http://"+dialAddr, plaintext) // NOSONAR: dialAddr is always loopback, this control plane calling its own local REST API in-process, never a real network HTTPS gap
	toolCaller, err := ai.NewToolCaller(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("build ai assistant tool caller: %w", err)
	}

	return ai.NewEngine(db, secretsManager, toolCaller, nil, brandName), nil
}
