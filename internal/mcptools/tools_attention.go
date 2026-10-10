package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// attentionOutput wraps the items because MCP structured output must be an object.
type attentionOutput struct {
	Items []attention.Item `json:"items"`
}

func registerAttentionTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_attention",
		Description: "List everything needing attention now, critical first: failing apps, offline nodes, expiring or stalled certificates, doctor warnings, and items waiting on a person (a CLI login, pending deploy approvals). Those must be approved by an operator in the dashboard, never through this server. Empty items means healthy. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, attentionOutput, error) {
		items, err := attention.Collect(ctx, client)
		if err != nil {
			return nil, attentionOutput{}, fmt.Errorf("get attention items: %w", err)
		}
		return nil, attentionOutput{Items: items}, nil
	})
}
