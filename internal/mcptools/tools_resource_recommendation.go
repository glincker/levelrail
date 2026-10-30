package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerResourceRecommendationTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_resource_recommendation",
		Description: "Suggest memory and CPU limits for an app from its own p95/p99 usage and current limits, weighing OOM kills heavily. Deterministic, no external model. Read only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in resourceRecommendationInput) (*mcp.CallToolResult, apiclient.ResourceRecommendationResource, error) {
		result, err := client.GetAppResourceRecommendation(ctx, in.Name)
		if err != nil {
			return nil, apiclient.ResourceRecommendationResource{}, fmt.Errorf("get resource recommendation for app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_database_resource_recommendation",
		Description: "Suggest memory and CPU limits for a managed database from its own p95/p99 usage and current limits. Same engine as get_resource_recommendation. Read only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in databaseResourceRecommendationInput) (*mcp.CallToolResult, apiclient.ResourceRecommendationResource, error) {
		result, err := client.GetDatabaseResourceRecommendation(ctx, in.Name)
		if err != nil {
			return nil, apiclient.ResourceRecommendationResource{}, fmt.Errorf("get resource recommendation for database %q: %w", in.Name, err)
		}
		return nil, result, nil
	})
}

type resourceRecommendationInput struct {
	Name string `json:"name" jsonschema:"the app's name"`
}

type databaseResourceRecommendationInput struct {
	Name string `json:"name" jsonschema:"the database's name"`
}
