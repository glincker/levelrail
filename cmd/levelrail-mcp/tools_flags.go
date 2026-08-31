package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerFeatureFlagTools registers only the read side of feature
// flags. Flags gate live behavior in a running app with no redeploy, so
// a write tool here would let an AI assistant flip production behavior
// unattended; that's deliberately left to the CLI/dashboard, both of
// which put a human in the loop before a change takes effect.
func registerFeatureFlagTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_feature_flags",
		Description: "List an app's feature flags: key, name, enabled state, and rollout percentage. Read-only; does not create, edit, or toggle a flag.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.FeatureFlagResource, error) {
		flags, err := client.ListFeatureFlags(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list feature flags for app %q: %w", in.Name, err)
		}
		return nil, flags, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_feature_flag",
		Description: "Get one feature flag's full metadata: key, name, description, enabled state, rollout percentage. Read-only; does not create, edit, or toggle a flag.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in featureFlagInput) (*mcp.CallToolResult, apiclient.FeatureFlagResource, error) {
		flag, err := client.GetFeatureFlag(ctx, in.Name, in.ID)
		if err != nil {
			return nil, apiclient.FeatureFlagResource{}, fmt.Errorf("get feature flag %q for app %q: %w", in.ID, in.Name, err)
		}
		return nil, flag, nil
	})
}

type featureFlagInput struct {
	Name string `json:"name" jsonschema:"the app's name that owns the flag"`
	ID   string `json:"id" jsonschema:"the flag's id"`
}
