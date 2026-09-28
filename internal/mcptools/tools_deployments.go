package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listDeploymentsInput struct {
	Status      string `json:"status,omitempty" jsonschema:"comma-separated: building, ready, failed, canceled, rolled_back, superseded, held"`
	App         string `json:"app,omitempty" jsonschema:"only this app"`
	Branch      string `json:"branch,omitempty" jsonschema:"only this branch"`
	Trigger     string `json:"trigger,omitempty" jsonschema:"comma-separated: git push, manual, rollback, api, preview"`
	Environment string `json:"environment,omitempty" jsonschema:"environment name or id"`
	Since       string `json:"since,omitempty" jsonschema:"RFC3339 time or a duration ago such as 24h or 7d"`
	Until       string `json:"until,omitempty" jsonschema:"RFC3339 time or a duration ago"`
	Query       string `json:"q,omitempty" jsonschema:"search commit message, commit sha prefix and app name"`
	Live        bool   `json:"live,omitempty" jsonschema:"only the release currently serving each app"`
	PR          int    `json:"pr,omitempty" jsonschema:"only previews of this pull request number"`
	Limit       int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
	Cursor      string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous page"`
}

type deploymentsSummaryInput struct {
	Window string `json:"window,omitempty" jsonschema:"counting window such as 24h or 7d, at most 30d; default 24h"`
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func registerDeploymentTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_deployments",
		Description: "List deployments across every app the caller can read, newest first, with status, trigger, commit, branch, image digest, duration and rollback links. Filter by status, app, branch, trigger, environment, time range, text, live or pull request; page with cursor. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listDeploymentsInput) (*mcp.CallToolResult, apiclient.DeploymentList, error) {
		opts := apiclient.DeploymentListOptions{
			Statuses: splitList(in.Status), Triggers: splitList(in.Trigger), App: in.App, Branch: in.Branch,
			Environment: in.Environment, Since: in.Since, Until: in.Until, Query: in.Query,
			Live: in.Live, PR: in.PR, Limit: in.Limit, Cursor: in.Cursor,
		}
		list, err := client.ListDeployments(ctx, opts)
		if err != nil {
			return nil, apiclient.DeploymentList{}, fmt.Errorf("list deployments: %w", err)
		}
		if list.Items == nil {
			list.Items = []apiclient.DeploymentResource{}
		}
		return nil, list, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "deployments_summary",
		Description: "Summarize deployments across the apps the caller can read: counts by status in a window, in-progress and needs-attention counts, 24h failure rate, median and p95 duration, and deploys per day for 14 days. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deploymentsSummaryInput) (*mcp.CallToolResult, apiclient.DeploymentSummary, error) {
		s, err := client.GetDeploymentsSummary(ctx, in.Window)
		if err != nil {
			return nil, apiclient.DeploymentSummary{}, fmt.Errorf("deployments summary: %w", err)
		}
		return nil, s, nil
	})
}
