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
		Description: "List everything that needs attention now, critical first: failing apps, offline nodes, expired or expiring certificates, and doctor warnings or failures. Empty items means healthy. Read-only; same list as 'levelrail-cli attention'.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, attentionOutput, error) {
		items, err := attention.Collect(ctx, client)
		if err != nil {
			return nil, attentionOutput{}, fmt.Errorf("get attention items: %w", err)
		}
		return nil, attentionOutput{Items: items}, nil
	})
}
