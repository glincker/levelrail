package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerDiagnosticTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "diagnose_app_failure",
		Description: "Explain why an app's most recent deploy attempt failed, or why it's crashlooping: a deterministic pattern match over already-collected signals (deploy attempt error, reconcile conditions, crashloop state, recent logs), never a call to an external model. Read-only, changes nothing. Pass deploy_id to diagnose a specific past attempt instead of the newest one.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in diagnoseAppInput) (*mcp.CallToolResult, apiclient.DiagnosisResource, error) {
		result, err := client.DiagnoseApp(ctx, in.Name, in.DeployID)
		if err != nil {
			return nil, apiclient.DiagnosisResource{}, fmt.Errorf("diagnose app %q: %w", in.Name, err)
		}
		return nil, result, nil
	})
}

type diagnoseAppInput struct {
	Name     string `json:"name" jsonschema:"the app's name"`
	DeployID string `json:"deploy_id,omitempty" jsonschema:"diagnose this specific past deploy attempt instead of the app's newest one"`
}
