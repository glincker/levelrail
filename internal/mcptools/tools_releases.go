package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type releaseHistoryInput struct {
	Channel string `json:"channel,omitempty" jsonschema:"stable, beta or all"`
}

type rollbackPlanInput struct {
	Version string `json:"version" jsonschema:"release tag, e.g. v1.2.3"`
}

// asObject returns v as a generic object so tools/list carries no large
// output schema for these read-only views.
func asObject(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// registerReleaseTools exposes release history and the rollback plan as
// read-only views. Applying a rollback is host-side only and is
// deliberately not a tool.
func registerReleaseTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_releases",
		Description: "Last 5 control plane releases with a database schema compatibility verdict each. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in releaseHistoryInput) (*mcp.CallToolResult, map[string]any, error) {
		res, err := client.GetReleaseHistory(ctx, in.Channel)
		if err != nil {
			return nil, nil, fmt.Errorf("list releases: %w", err)
		}
		out, err := asObject(res)
		return nil, out, err
	})

	addTool(server, &mcp.Tool{
		Name:        "get_rollback_plan",
		Description: "Preview a control plane rollback: schema verdict, checks, downtime, data loss. Applies nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in rollbackPlanInput) (*mcp.CallToolResult, map[string]any, error) {
		res, err := client.GetRollbackPlan(ctx, in.Version)
		if err != nil {
			return nil, nil, fmt.Errorf("get rollback plan for %q: %w", in.Version, err)
		}
		out, err := asObject(res)
		return nil, out, err
	})
}
