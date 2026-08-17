package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerServiceTemplateTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_service_templates",
		Description: "List the service template catalog (id, name, category, slogan), without each entry's full Compose body. Use get_service_template for that.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.ServiceTemplateListItem, error) {
		templates, err := client.ListServiceTemplates(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list service templates: %w", err)
		}
		return nil, templates, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_service_template",
		Description: "Get one service template catalog entry, including its full compose.yaml body, ready to pass to deploy_compose.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in serviceTemplateIDInput) (*mcp.CallToolResult, apiclient.ServiceTemplateDetail, error) {
		template, err := client.GetServiceTemplate(ctx, in.ID)
		if err != nil {
			return nil, apiclient.ServiceTemplateDetail{}, fmt.Errorf("get service template %q: %w", in.ID, err)
		}
		return nil, template, nil
	})
}

type serviceTemplateIDInput struct {
	ID string `json:"id" jsonschema:"the service template's id, from list_service_templates"`
}
