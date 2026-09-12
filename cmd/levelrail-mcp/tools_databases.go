package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDatabaseTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_databases",
		Description: "List every managed database on the control plane: name, engine, version.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DatabaseResource, error) {
		databases, err := client.ListDatabases(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list databases: %w", err)
		}
		return nil, databases, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_database",
		Description: "Get one managed database's current desired state: engine, version.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseNameInput) (*mcp.CallToolResult, apiclient.DatabaseResource, error) {
		database, err := client.GetDatabase(ctx, in.Name)
		if err != nil {
			return nil, apiclient.DatabaseResource{}, fmt.Errorf("get database %q: %w", in.Name, err)
		}
		return nil, database, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_database_engines",
		Description: "List every database engine this control plane can create right now: id, display label, and default version. Backed by the embedded database engine registry, not a hardcoded list, so this reflects exactly what the creation wizard's engine picker offers.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DatabaseEngineResource, error) {
		engines, err := client.ListDatabaseEngines(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list database engines: %w", err)
		}
		return nil, engines, nil
	})
}

type databaseNameInput struct {
	Name string `json:"name" jsonschema:"the database's name"`
}
