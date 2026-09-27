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
		Description: "Explain why an app's latest deploy failed or it is crashlooping, by deterministic pattern match over deploy errors, conditions and logs. Returns typed causes (WRONG_PORT, OOM_KILLED, MISSING_ENV) with evidence, fixes and recent changes. Read-only. Pass deploy_id for a specific past attempt.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in diagnoseAppInput) (*mcp.CallToolResult, apiclient.DiagnosisResource, error) {
		result, err := client.DiagnoseApp(ctx, in.Name, in.DeployID)
		if err != nil {
			return nil, apiclient.DiagnosisResource{}, fmt.Errorf("diagnose app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "preflight_app",
		Description: "Run read-only pre-deploy checks on an app's stored config: DNS, port, disk and memory headroom, image pull, repo access, env, mounts. Each check reports pass, warn or fail with a fix hint. Pass required_env to verify variable names are set.",
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
