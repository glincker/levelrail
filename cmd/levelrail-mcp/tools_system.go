package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerSystemTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_system_doctor",
		Description: "Run the control plane's local preflight health check: Docker daemon reachability, disk space and write access, ingress port availability, database reachability. Read-only, changes nothing; the same report 'levelrail-cli doctor' prints.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.SystemDoctorResource, error) {
		report, err := client.GetSystemDoctor(ctx)
		if err != nil {
			return nil, apiclient.SystemDoctorResource{}, fmt.Errorf("get system doctor report: %w", err)
		}
		return nil, report, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_onboarding_status",
		Description: "Get whether the control plane's first-run onboarding flow has been completed. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.OnboardingStateResource, error) {
		state, err := client.GetOnboardingState(ctx)
		if err != nil {
			return nil, apiclient.OnboardingStateResource{}, fmt.Errorf("get onboarding status: %w", err)
		}
		return nil, state, nil
	})
}
