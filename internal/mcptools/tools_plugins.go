package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const pluginsGroup = "plugins"

type pluginNameInput struct {
	Name string `json:"name" jsonschema:"the plugin's name"`
}

func registerPluginTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_plugins",
		Description: "List installed plugins with publisher, version, requested and approved abilities, lifecycle state, health, signature verification status and the fix the dashboard would offer. Needs a root token and the plugins experimental feature. Read only: install, approve, upgrade and removal stay in the dashboard and CLI.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.Plugin, error) {
		plugins, err := client.ListPlugins(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list plugins: %w", err)
		}
		return nil, plugins, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_plugin",
		Description: "Get one installed plugin: its signed manifest, abilities requested and approved, outbound hosts, health, verification status, last error and contributed panels. Read only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pluginNameInput) (*mcp.CallToolResult, apiclient.Plugin, error) {
		p, err := client.GetPlugin(ctx, in.Name)
		if err != nil {
			return nil, apiclient.Plugin{}, fmt.Errorf("get plugin %q: %w", in.Name, err)
		}
		return nil, p, nil
	})
}
