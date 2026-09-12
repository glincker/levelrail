package main

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAppConfigTools(server *mcp.Server, client *apiclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_git_source",
		Description: "Get an app's connected git source, if any: repo URL, branch, build type/path, the webhook URL to configure at the provider, and whether a deploy token is stored (never the token or webhook secret itself). Explains why a push isn't triggering a deploy. Read-only; does not connect, edit, or disconnect a source.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.GitSourceResource, error) {
		source, err := client.GetGitSource(ctx, in.Name)
		if err != nil {
			return nil, apiclient.GitSourceResource{}, fmt.Errorf("get git source for app %q: %w", in.Name, err)
		}
		return nil, source, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_app_hook_runs",
		Description: "Get the most recent outcome of an app's pre-deploy and post-deploy hooks, if configured: exit code, success, and captured output for each. A hook that has never run comes back absent, not an empty result. Read-only; does not run a hook.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppHookRunsResource, error) {
		runs, err := client.GetAppHookRuns(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppHookRunsResource{}, fmt.Errorf("get hook runs for app %q: %w", in.Name, err)
		}
		return nil, runs, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_domain_tls_cert_status",
		Description: "Get whether one of an app's domains has an operator-supplied (BYO) TLS certificate uploaded in place of Caddy's automatic ACME issuance, and when it was uploaded/expires. Neither the certificate nor the private key is ever returned. Read-only; does not upload or clear a certificate.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appDomainInput) (*mcp.CallToolResult, apiclient.DomainTLSCertResource, error) {
		status, err := client.GetDomainTLSCert(ctx, in.Name, in.Domain)
		if err != nil {
			return nil, apiclient.DomainTLSCertResource{}, fmt.Errorf("get tls cert status for app %q domain %q: %w", in.Name, in.Domain, err)
		}
		return nil, status, nil
	})
}

type appDomainInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Domain string `json:"domain" jsonschema:"the domain to check, e.g. app.example.com"`
}
