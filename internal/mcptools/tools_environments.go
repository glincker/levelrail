package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerEnvironmentTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_environments",
		Description: "List a project's staging/production-style environment labels: name and whether it's protected (deploying into a protected environment requires the caller to pass confirm=true to deploy_app/rollback_app). Read-only; does not create, edit, or delete an environment.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projectIDInput) (*mcp.CallToolResult, []apiclient.EnvironmentResource, error) {
		environments, err := client.ListEnvironments(ctx, in.ProjectID)
		if err != nil {
			return nil, nil, fmt.Errorf("list environments for project %q: %w", in.ProjectID, err)
		}
		return nil, environments, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "preview_clone_environment",
		Description: "Show what clone_environment would create: one entry per app tagged with the source environment (its suggested new name, current image, domains that will NOT be copied, declared secret env var names, scheduled task count), plus the environment's own shared env var keys. Read-only; creates nothing.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cloneEnvironmentPreviewInput) (*mcp.CallToolResult, apiclient.EnvironmentClonePreviewResource, error) {
		preview, err := client.PreviewEnvironmentClone(ctx, in.EnvironmentID, in.NewEnvironmentName)
		if err != nil {
			return nil, apiclient.EnvironmentClonePreviewResource{}, fmt.Errorf("preview clone of environment %q: %w", in.EnvironmentID, err)
		}
		return nil, preview, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "clone_environment",
		Description: "Clone a whole environment: creates a new environment in the same project, then a real, deployed copy of every app tagged with the source environment (image, env vars, resources, health checks, volumes, bind mounts, labels, scheduled tasks). Domains and host port pins are never copied (assign new ones per app if needed). Secret values (per-app and shared) are declared on the clone with no value set unless copy_secret_values is true; every declared secret otherwise behaves like a brand-new required secret. Asynchronous: use get_app_status on each cloned app to watch it converge.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in cloneEnvironmentInput) (*mcp.CallToolResult, apiclient.EnvironmentCloneResultResource, error) {
		result, err := client.CloneEnvironment(ctx, in.EnvironmentID, apiclient.EnvironmentCloneRequest{
			NewEnvironmentName: in.NewEnvironmentName,
			CopySecretValues:   in.CopySecretValues,
		})
		if err != nil {
			return nil, apiclient.EnvironmentCloneResultResource{}, fmt.Errorf("clone environment %q: %w", in.EnvironmentID, err)
		}
		return nil, result, nil
	})
}

type projectIDInput struct {
	ProjectID string `json:"project_id" jsonschema:"the project's id, from list_projects"`
}

type cloneEnvironmentPreviewInput struct {
	EnvironmentID      string `json:"environment_id" jsonschema:"the source environment's id, from list_environments"`
	NewEnvironmentName string `json:"new_environment_name" jsonschema:"the name a same-request clone_environment call would give the new environment"`
}

type cloneEnvironmentInput struct {
	EnvironmentID      string `json:"environment_id" jsonschema:"the source environment's id, from list_environments"`
	NewEnvironmentName string `json:"new_environment_name" jsonschema:"name for the new environment"`
	CopySecretValues   bool   `json:"copy_secret_values,omitempty" jsonschema:"true to also copy real secret values (per-app and shared) onto the clone; omit or false leaves every secret declared but unset, the safe default"`
}
