package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bulkAppsInput struct {
	Action      string   `json:"action" jsonschema:"one of redeploy, restart, stop, start, add-tag, remove-tag, set-environment, move-to-project"`
	Names       []string `json:"names,omitempty" jsonschema:"app names to act on; alternatively select by tag or environment"`
	Tag         string   `json:"tag,omitempty" jsonschema:"select every app carrying this tag"`
	Environment string   `json:"environment,omitempty" jsonschema:"select every app in this environment (name or ID)"`
	Value       string   `json:"value,omitempty" jsonschema:"the tag, environment or project ID for add-tag, remove-tag, set-environment and move-to-project"`
	DryRun      bool     `json:"dry_run,omitempty" jsonschema:"list what would be applied and change nothing"`
}

type bulkDeleteAppsInput struct {
	Names        []string `json:"names,omitempty" jsonschema:"app names to delete"`
	Tag          string   `json:"tag,omitempty" jsonschema:"select every app carrying this tag"`
	Environment  string   `json:"environment,omitempty" jsonschema:"select every app in this environment (name or ID)"`
	ConfirmNames []string `json:"confirm_names,omitempty" jsonschema:"must list exactly the apps that will be deleted; run once with dry_run true to see them"`
	DryRun       bool     `json:"dry_run,omitempty" jsonschema:"list what would be deleted and change nothing"`
}

func registerBulkAppTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "bulk_apps",
		Description: "Apply one action to many apps at once, selected by names, tag or environment. Each app is authorized separately: denied apps are reported per item, not hidden, and do not fail the batch. Use dry_run first to see the targets. Apps in protected environments are skipped by redeploy. Deleting is a separate tool, bulk_delete_apps.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bulkAppsInput) (*mcp.CallToolResult, apiclient.BulkAppsResponse, error) {
		if in.Action == "delete" {
			return nil, apiclient.BulkAppsResponse{}, fmt.Errorf("bulk apps: use bulk_delete_apps to delete")
		}
		resp, err := client.BulkApps(ctx, apiclient.BulkAppsRequest{
			Action: in.Action, Names: in.Names, Tag: in.Tag, Environment: in.Environment, Value: in.Value, DryRun: in.DryRun,
		})
		if err != nil {
			return nil, apiclient.BulkAppsResponse{}, fmt.Errorf("bulk apps %q: %w", in.Action, err)
		}
		return nil, resp, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "bulk_delete_apps",
		Description: "Permanently delete many apps at once. Requires confirm_names to equal the exact target list; call with dry_run true first and confirm the list with the user. Each deletion is audit logged per app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bulkDeleteAppsInput) (*mcp.CallToolResult, apiclient.BulkAppsResponse, error) {
		resp, err := client.BulkApps(ctx, apiclient.BulkAppsRequest{
			Action: "delete", Names: in.Names, Tag: in.Tag, Environment: in.Environment, ConfirmNames: in.ConfirmNames, DryRun: in.DryRun,
		})
		if err != nil {
			return nil, apiclient.BulkAppsResponse{}, fmt.Errorf("bulk delete apps: %w", err)
		}
		return nil, resp, nil
	})
}
