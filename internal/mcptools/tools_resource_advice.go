package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const resourceAdviceTool = "resource_advice"

func registerResourceAdviceTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        resourceAdviceTool,
		Description: "Beta resource advisor findings: limits, memory growth, idle apps and databases, engine tuning, disk and host memory pressure, each with evidence and a suggested action.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in resourceAdviceInput) (*mcp.CallToolResult, apiclient.AdvisorFindings, error) {
		out, err := client.ListAdvisorFindings(ctx, apiclient.AdvisorFindingQuery{Resource: in.Resource, Severity: in.Severity})
		if err != nil {
			return nil, apiclient.AdvisorFindings{}, fmt.Errorf("list resource advice: %w", err)
		}
		return nil, out, nil
	})
}

type resourceAdviceInput struct {
	Resource string `json:"resource,omitempty" jsonschema:"only this resource, as app:NAME, database:NAME or node:ID"`
	Severity string `json:"severity,omitempty" jsonschema:"minimum severity: info, suggest or warn"`
}
