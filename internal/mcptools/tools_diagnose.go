package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDiagnosticTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "diagnose_app_failure",
		Description: "Why an app's latest deploy failed or it crashloops: typed causes (WRONG_PORT, OOM_KILLED) with evidence and fixes. Pass deploy_id for a past attempt.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in diagnoseAppInput) (*mcp.CallToolResult, apiclient.DiagnosisResource, error) {
		result, err := client.DiagnoseApp(ctx, in.Name, in.DeployID)
		if err != nil {
			return nil, apiclient.DiagnosisResource{}, fmt.Errorf("diagnose app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "preflight_app",
		Description: "Read-only pre-deploy checks of an app's config: DNS, port, disk, memory, image pull, repo, env, mounts, each with a fix hint. Pass required_env to verify names.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in preflightAppInput) (*mcp.CallToolResult, apiclient.PreflightReport, error) {
		report, err := client.PreflightApp(ctx, in.Name, in.RequiredEnv)
		if err != nil {
			return nil, apiclient.PreflightReport{}, fmt.Errorf("preflight app %q: %w", in.Name, err)
		}
		return nil, report, nil
	})
}

type preflightAppInput struct {
	Name        string   `json:"name" jsonschema:"the app's name"`
	RequiredEnv []string `json:"required_env,omitempty" jsonschema:"env var names that must be set for the app to start"`
}

type diagnoseAppInput struct {
	Name     string `json:"name" jsonschema:"the app's name"`
	DeployID string `json:"deploy_id,omitempty" jsonschema:"diagnose this specific past deploy attempt instead of the app's newest one"`
}
