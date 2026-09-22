package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDeployApprovalTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_deploy_approvals",
		Description: "List pending (or, with status 'all', every) two-person approval gate on a deploy/promote into a protected environment. status defaults to 'pending' when omitted; service filters to one app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listDeployApprovalsInput) (*mcp.CallToolResult, []apiclient.DeployApprovalResource, error) {
		approvals, err := client.ListDeployApprovals(ctx, in.Status, in.Service)
		if err != nil {
			return nil, nil, fmt.Errorf("list deploy approvals: %w", err)
		}
		return nil, approvals, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_deploy_approval",
		Description: "Get one deploy approval by ID: who requested it, what it would deploy, and its current status.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployApprovalIDInput) (*mcp.CallToolResult, apiclient.DeployApprovalResource, error) {
		a, err := client.GetDeployApproval(ctx, in.ID)
		if err != nil {
			return nil, apiclient.DeployApprovalResource{}, fmt.Errorf("get deploy approval %q: %w", in.ID, err)
		}
		return nil, a, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "approve_deploy_approval",
		Description: "Approve a pending deploy approval: the gated deploy/promote actually runs through the normal reconcile path once this succeeds. Fails if the caller is the same actor who requested it, or if it's no longer pending.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deployApprovalIDInput) (*mcp.CallToolResult, apiclient.DeployApprovalDecisionResult, error) {
		result, err := client.ApproveDeployApproval(ctx, in.ID)
		if err != nil {
			return nil, apiclient.DeployApprovalDecisionResult{}, fmt.Errorf("approve deploy approval %q: %w", in.ID, err)
		}
		return nil, result, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reject_deploy_approval",
		Description: "Reject a pending deploy approval: the app's desired state is left untouched, it never reaches reconcile.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in rejectDeployApprovalInput) (*mcp.CallToolResult, apiclient.DeployApprovalResource, error) {
		a, err := client.RejectDeployApproval(ctx, in.ID, in.Reason)
		if err != nil {
			return nil, apiclient.DeployApprovalResource{}, fmt.Errorf("reject deploy approval %q: %w", in.ID, err)
		}
		return nil, a, nil
	})
}

type listDeployApprovalsInput struct {
	Status  string `json:"status,omitempty" jsonschema:"'pending' (default), 'all', or one of approved/rejected/expired"`
	Service string `json:"service,omitempty" jsonschema:"filter to one app name; omit for every app"`
}

type deployApprovalIDInput struct {
	ID string `json:"id" jsonschema:"the deploy approval's ID"`
}

type rejectDeployApprovalInput struct {
	ID     string `json:"id" jsonschema:"the deploy approval's ID"`
	Reason string `json:"reason,omitempty" jsonschema:"optional note explaining the rejection"`
}
