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
		Description: "Installed plugins: publisher, version, abilities, state, health, signature status, suggested fix. Root token and plugins feature needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.Plugin, error) {
		plugins, err := client.ListPlugins(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list plugins: %w", err)
		}
		return nil, plugins, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_marketplace_plugins",
		Description: "Plugins in the signed marketplace indexes: publisher, version, abilities, digest, key trust, state. Root token and plugins feature needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.MarketplaceEntry, error) {
		cat, err := client.MarketplaceCatalog(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list marketplace plugins: %w", err)
		}
		return nil, cat.Entries, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_plugin",
		Description: "Get one installed plugin: its manifest, abilities requested and approved, outbound hosts, health, verification, last error and panels.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in pluginNameInput) (*mcp.CallToolResult, apiclient.Plugin, error) {
		p, err := client.GetPlugin(ctx, in.Name)
		if err != nil {
			return nil, apiclient.Plugin{}, fmt.Errorf("get plugin %q: %w", in.Name, err)
		}
		return nil, p, nil
	})
}
