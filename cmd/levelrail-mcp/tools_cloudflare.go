package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerCloudflareTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_cloudflare_tunnel_status",
		Description: "Get this control plane's Cloudflare Tunnel connection status: enabled, whether a token is stored, and the tunnel's current running status. The token itself is never returned. Read-only; does not connect, edit, or disconnect the tunnel.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.CloudflareTunnelResource, error) {
		status, err := client.GetCloudflareTunnel(ctx)
		if err != nil {
			return nil, apiclient.CloudflareTunnelResource{}, fmt.Errorf("get cloudflare tunnel status: %w", err)
		}
		return nil, status, nil
	})
}
