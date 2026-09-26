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
		Description: "Explain why an app's most recent deploy attempt failed, or why it's crashlooping: a deterministic pattern match over already-collected signals (deploy attempt error, reconcile conditions, crashloop state, recent logs), never a call to an external model. Read-only, changes nothing. The result carries typed causes (for example WRONG_PORT, OOM_KILLED, MISSING_ENV) with evidence and numbered fixes; each fix lists the exact field changes it would make, which an operator can apply from the dashboard or CLI. Pass deploy_id to diagnose a specific past attempt instead of the newest one.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in diagnoseAppInput) (*mcp.CallToolResult, apiclient.DiagnosisResource, error) {
		result, err := client.DiagnoseApp(ctx, in.Name, in.DeployID)
		if err != nil {
			return nil, apiclient.DiagnosisResource{}, fmt.Errorf("diagnose app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "preflight_app",
		Description: "Run read-only pre-deploy checks against an existing app's stored configuration: DNS points at this server, host port free, disk and memory headroom, image pullable, repository and branch reachable, required env present, mount paths valid, GPU available. Each check reports pass, warn or fail with a reason and a fix hint. Deterministic, changes nothing. Pass required_env to also verify those variable names are set.",
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
