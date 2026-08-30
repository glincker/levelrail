package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerPreviewTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_preview_environments",
		Description: "List an app's active pull-request preview environments: PR number, branch, head commit, domain, and status. Read-only; does not create or tear down a preview.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, []apiclient.PreviewEnvironmentResource, error) {
		previews, err := client.ListPreviewEnvironments(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("list preview environments for app %q: %w", in.Name, err)
		}
		return nil, previews, nil
	})
}
