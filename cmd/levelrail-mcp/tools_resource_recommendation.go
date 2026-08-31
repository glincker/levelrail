package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerResourceRecommendationTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_resource_recommendation",
		Description: "Suggest memory and CPU limits for an app based on its own historical usage (p95/p99 over a lookback window) and current limits: a deterministic engine, never a call to an external model. Weighs a real OOM-kill signal heavily when one was found in recent logs. Read-only, changes nothing, and the suggestion is never applied automatically.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in resourceRecommendationInput) (*mcp.CallToolResult, apiclient.ResourceRecommendationResource, error) {
		result, err := client.GetAppResourceRecommendation(ctx, in.Name)
		if err != nil {
			return nil, apiclient.ResourceRecommendationResource{}, fmt.Errorf("get resource recommendation for app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})
}

type resourceRecommendationInput struct {
	Name string `json:"name" jsonschema:"the app's name"`
}
