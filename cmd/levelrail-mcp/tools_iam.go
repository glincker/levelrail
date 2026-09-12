package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerIAMTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_iam_policies",
		Description: "List every IAM policy on the control plane: resource-scoped Allow/Deny documents attachable to a user or token, additive on top of a token's flat abilities. Read-only; does not create, edit, delete, attach, or detach a policy.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.PolicyResource, error) {
		policies, err := client.ListPolicies(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list iam policies: %w", err)
		}
		return nil, policies, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_iam_policy",
		Description: "Get one IAM policy's full document (name, description, the Allow/Deny statements themselves). Read-only; does not create, edit, delete, attach, or detach a policy.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in policyIDInput) (*mcp.CallToolResult, apiclient.PolicyResource, error) {
		policy, err := client.GetPolicy(ctx, in.ID)
		if err != nil {
			return nil, apiclient.PolicyResource{}, fmt.Errorf("get iam policy %q: %w", in.ID, err)
		}
		return nil, policy, nil
	})
}

type policyIDInput struct {
	ID string `json:"id" jsonschema:"the policy's id"`
}
