package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type searchInput struct {
	Query string `json:"query" jsonschema:"scopes app: env: tag: node: kind: then free text, e.g. 'env:prod api'"`
	Kind  string `json:"kind,omitempty" jsonschema:"limit to one kind: app, database, node, domain, template, deployment, setting, doc, action"`
	Limit int    `json:"limit,omitempty" jsonschema:"hits to return, default 20, max 100"`
}

type searchActionsInput struct {
	Query string `json:"query" jsonschema:"what to do and to what, e.g. 'restart api'"`
	Limit int    `json:"limit,omitempty" jsonschema:"hits to return, default 20, max 100"`
}

type searchOutput struct {
	Hits  []apiclient.SearchHit `json:"hits"`
	Total int                   `json:"total"`
}

func registerSearchTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search apps, databases, nodes, domains, templates, deployments, settings and docs at once. Returns only what the token may see.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
		resp, err := client.Search(ctx, in.Query, in.Kind, in.Limit, "")
		if err != nil {
			return nil, searchOutput{}, fmt.Errorf("search: %w", err)
		}
		return nil, searchOutput{Hits: resp.Hits, Total: resp.Total}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "search_actions",
		Description: "Find actions (deploy, restart, logs, rollback, backup now, add domain) the token may run on a named app or database. Lists only, runs nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in searchActionsInput) (*mcp.CallToolResult, searchOutput, error) {
		resp, err := client.Search(ctx, in.Query, "action", in.Limit, "")
		if err != nil {
			return nil, searchOutput{}, fmt.Errorf("search actions: %w", err)
		}
		return nil, searchOutput{Hits: resp.Hits, Total: resp.Total}, nil
	})
}
