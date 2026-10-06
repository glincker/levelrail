package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerRoleTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_roles",
		Description: "List stored roles: id, name, abilities, visibility (all or granted), builtin, user_count. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.RoleResource, error) {
		roles, err := client.ListRoles(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list roles: %w", err)
		}
		return nil, roles, nil
	})
}
