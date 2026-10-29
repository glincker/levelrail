package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCloudflareTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_cloudflare_tunnel_status",
		Description: "Cloudflare Tunnel status: enabled, whether a token is stored, and the tunnel's running status. The token is never returned.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.CloudflareTunnelResource, error) {
		status, err := client.GetCloudflareTunnel(ctx)
		if err != nil {
			return nil, apiclient.CloudflareTunnelResource{}, fmt.Errorf("get cloudflare tunnel status: %w", err)
		}
		return nil, status, nil
	})
}
