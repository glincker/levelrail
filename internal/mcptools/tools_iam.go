package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerIAMTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_iam_policies",
		Description: "List every IAM policy on the control plane: resource-scoped Allow/Deny documents attachable to a user or token, additive on top of a token's flat abilities.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.PolicyResource, error) {
		policies, err := client.ListPolicies(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list iam policies: %w", err)
		}
		return nil, policies, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_iam_policy",
		Description: "Get one IAM policy's full document (name, description, the Allow/Deny statements themselves).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in policyIDInput) (*mcp.CallToolResult, apiclient.PolicyResource, error) {
		policy, err := client.GetPolicy(ctx, in.ID)
		if err != nil {
			return nil, apiclient.PolicyResource{}, fmt.Errorf("get iam policy %q: %w", in.ID, err)
		}
		return nil, policy, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_policy_templates",
		Description: "List ready-made IAM policy templates (read-only, guest-one-environment, deployer-nonprod, ai-operator-nonprod, production-approver) with their parameters and documents.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.PolicyTemplateList, error) {
		list, err := client.ListPolicyTemplates(ctx)
		if err != nil {
			return nil, apiclient.PolicyTemplateList{}, fmt.Errorf("list policy templates: %w", err)
		}
		return nil, list, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "simulate_iam_access",
		Description: "Can a user or token do an ability on a resource (write on app:web)? Allow or deny with the deciding and matching statements.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in simulateIAMInput) (*mcp.CallToolResult, apiclient.IAMSimulation, error) {
		sim, err := client.SimulateIAM(ctx, in.PrincipalType, in.PrincipalID, in.Action, in.Resource)
		if err != nil {
			return nil, apiclient.IAMSimulation{}, fmt.Errorf("simulate iam access: %w", err)
		}
		return nil, sim, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_iam_effective_permissions",
		Description: "Show what a user or token can do on every app and database, grouped by ability, with how much comes from policies.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in principalInput) (*mcp.CallToolResult, apiclient.IAMEffective, error) {
		eff, err := client.IAMEffectivePermissions(ctx, in.PrincipalType, in.PrincipalID)
		if err != nil {
			return nil, apiclient.IAMEffective{}, fmt.Errorf("get iam effective permissions: %w", err)
		}
		return nil, eff, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "analyze_iam_policies",
		Description: "Run static checks over IAM policies, attachments and tokens: a score and findings with severity and a one line fix.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.IAMAnalysis, error) {
		res, err := client.AnalyzeIAM(ctx)
		if err != nil {
			return nil, apiclient.IAMAnalysis{}, fmt.Errorf("analyze iam policies: %w", err)
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "suggest_least_privilege",
		Description: "Suggest narrower abilities or revocation for each token from audit log usage. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in suggestInput) (*mcp.CallToolResult, apiclient.IAMSuggestions, error) {
		res, err := client.IAMSuggestions(ctx, in.Days)
		if err != nil {
			return nil, apiclient.IAMSuggestions{}, fmt.Errorf("suggest least privilege: %w", err)
		}
		return nil, res, nil
	})
}

type suggestInput struct {
	Days int `json:"days,omitempty" jsonschema:"window in days"`
}

type policyIDInput struct {
	ID string `json:"id" jsonschema:"the policy's id"`
}

type principalInput struct {
	PrincipalType string `json:"principal_type" jsonschema:"user or token"`
	PrincipalID   string `json:"principal_id" jsonschema:"the user or token id"`
}

type simulateIAMInput struct {
	PrincipalType string `json:"principal_type" jsonschema:"user or token"`
	PrincipalID   string `json:"principal_id" jsonschema:"the user or token id"`
	Action        string `json:"action" jsonschema:"ability to test: read, read:sensitive, write, write:sensitive, deploy or root"`
	Resource      string `json:"resource" jsonschema:"resource identifier such as app:web or database:main"`
}
