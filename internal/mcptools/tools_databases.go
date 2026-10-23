package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDatabaseTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_databases",
		Description: "List every managed database on the control plane: name, engine, version.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DatabaseResource, error) {
		databases, err := client.ListDatabases(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list databases: %w", err)
		}
		return nil, databases, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_database",
		Description: "Get one managed database's current desired state: engine, version.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseNameInput) (*mcp.CallToolResult, apiclient.DatabaseResource, error) {
		database, err := client.GetDatabase(ctx, in.Name)
		if err != nil {
			return nil, apiclient.DatabaseResource{}, fmt.Errorf("get database %q: %w", in.Name, err)
		}
		return nil, database, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_database_engines",
		Description: "Database engines this control plane can create: id, label, default version.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.DatabaseEngineResource, error) {
		engines, err := client.ListDatabaseEngines(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list database engines: %w", err)
		}
		return nil, engines, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_external_databases",
		Description: "Databases connected in place or adopted, not run here: name, engine, host, port, last health probe. Never a password.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ExternalDatabase, error) {
		databases, err := client.ListExternalDatabases(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list external databases: %w", err)
		}
		return nil, databases, nil
	})
}

type databaseNameInput struct {
	Name string `json:"name" jsonschema:"the database's name"`
}
