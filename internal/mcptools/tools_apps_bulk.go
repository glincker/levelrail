package mcptools

import (
	"context"
	"fmt"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type bulkAppsInput struct {
	Action       string   `json:"action" jsonschema:"one of redeploy, restart, stop, start, add-tag, remove-tag, set-environment, move-to-project, scale, rollback, set-memory, set-cpu, set-env"`
	Names        []string `json:"names,omitempty" jsonschema:"app names to act on; alternatively select by tag or environment"`
	Tag          string   `json:"tag,omitempty" jsonschema:"select every app carrying this tag"`
	Environment  string   `json:"environment,omitempty" jsonschema:"select every app in this environment (name or ID)"`
	Project      string   `json:"project,omitempty" jsonschema:"select every app in this project ID"`
	Value        string   `json:"value,omitempty" jsonschema:"the tag, environment or project ID for add-tag, remove-tag, set-environment and move-to-project"`
	DryRun       bool     `json:"dry_run,omitempty" jsonschema:"list what would be applied and change nothing"`
	Replicas     *int     `json:"replicas,omitempty" jsonschema:"scale: replicas for apps without autoscale"`
	MinReplicas  *int     `json:"min_replicas,omitempty" jsonschema:"scale: autoscale minimum for autoscaled apps, with max_replicas"`
	MaxReplicas  *int     `json:"max_replicas,omitempty" jsonschema:"scale: autoscale maximum for autoscaled apps, with min_replicas"`
	MemoryBytes  *int64   `json:"memory_bytes,omitempty" jsonschema:"set-memory: memory limit in bytes"`
	NanoCPUs     *int64   `json:"nano_cpus,omitempty" jsonschema:"set-cpu: CPU limit in nano-CPUs (1000000000 is one core)"`
	EnvKey       string   `json:"env_key,omitempty" jsonschema:"set-env: variable name"`
	EnvValue     *string  `json:"env_value,omitempty" jsonschema:"set-env: variable value"`
	ConfirmCount int      `json:"confirm_count,omitempty" jsonschema:"number of target apps, needed to restart, stop, redeploy or roll back more apps than the confirm limit"`
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
		Description: "Apply one action to many apps selected by names, tag or environment. Each app is authorized separately. dry_run first (set-memory, set-cpu and set-env show before and after per app). Deleting: bulk_delete_apps.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in bulkAppsInput) (*mcp.CallToolResult, apiclient.BulkAppsResponse, error) {
		if in.Action == "delete" {
			return nil, apiclient.BulkAppsResponse{}, fmt.Errorf("bulk apps: use bulk_delete_apps to delete")
		}
		resp, err := client.BulkApps(ctx, apiclient.BulkAppsRequest{
			Action: in.Action, Names: in.Names, Tag: in.Tag, Environment: in.Environment, Project: in.Project, Value: in.Value, DryRun: in.DryRun,
			Replicas: in.Replicas, MinReplicas: in.MinReplicas, MaxReplicas: in.MaxReplicas,
			MemoryBytes: in.MemoryBytes, NanoCPUs: in.NanoCPUs, EnvKey: in.EnvKey, EnvValue: in.EnvValue, ConfirmCount: in.ConfirmCount,
		})
		if err != nil {
			return nil, apiclient.BulkAppsResponse{}, fmt.Errorf("bulk apps %q: %w", in.Action, err)
		}
		return nil, resp, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "bulk_delete_apps",
		Description: "Permanently delete many apps. confirm_names must equal the exact target list; run dry_run first and confirm with the user.",
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
