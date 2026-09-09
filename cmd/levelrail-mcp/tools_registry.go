package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRegistryCredentialTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_registry_credentials",
		Description: "List every private image registry credential connected to the control plane: host, username, and an expiry_status of healthy/expiring_soon/expired when an expiry was set. No password field, ever. Read-only; does not create, edit, delete, or test a credential.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.RegistryCredentialResource, error) {
		creds, err := client.ListRegistryCredentials(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list registry credentials: %w", err)
		}
		return nil, creds, nil
	})
}
