package mcptools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type deployPoliciesOutput struct {
	Policies []apiclient.DeployPolicy `json:"policies"`
}

// registerDeployPolicyTools adds the read-only view of deploy guardrails.
// Changing a policy or running a dry run stays with the dashboard and CLI.
func registerDeployPolicyTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_deploy_policies",
		Description: "List deploy guardrail rules per org, project and environment scope (enforce, warn or off).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, deployPoliciesOutput, error) {
		list, err := client.ListDeployPolicies(ctx)
		if err != nil {
			return nil, deployPoliciesOutput{}, fmt.Errorf("list deploy policies: %w", err)
		}
		return nil, deployPoliciesOutput{Policies: list}, nil
	})
}
