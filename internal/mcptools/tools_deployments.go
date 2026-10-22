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
	Since       string `json:"since,omitempty" jsonschema:"RFC3339 or duration ago (24h, 7d)"`
	Until       string `json:"until,omitempty" jsonschema:"RFC3339 or duration ago"`
	Query       string `json:"q,omitempty" jsonschema:"commit message, sha prefix or app name"`
	Live        bool   `json:"live,omitempty" jsonschema:"only releases now serving"`
	PR          int    `json:"pr,omitempty" jsonschema:"only this PR's previews"`
	Limit       int    `json:"limit,omitempty" jsonschema:"page size, default 50, max 200"`
	Cursor      string `json:"cursor,omitempty" jsonschema:"next_cursor of the prior page"`
	Fields      string `json:"fields,omitempty" jsonschema:"comma-separated keys to keep per item"`
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
		Description: "List deployments across readable apps, newest first. Filter by status, app, branch, trigger, environment, time, text, live or PR; page with cursor.",
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
		Description: "Summarize deployments across the apps the caller can read: counts by status in a window, in-progress and needs-attention counts, 24h failure rate, median and p95 duration, and deploys per day for 14 days.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in deploymentsSummaryInput) (*mcp.CallToolResult, apiclient.DeploymentSummary, error) {
		s, err := client.GetDeploymentsSummary(ctx, in.Window)
		if err != nil {
			return nil, apiclient.DeploymentSummary{}, fmt.Errorf("deployments summary: %w", err)
		}
		return nil, s, nil
	})
}
