package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerAppConfigTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_git_source",
		Description: "An app's git source: repo, branch, build type, webhook URL, whether a deploy token is stored (never the token). Explains missing push deploys.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.GitSourceResource, error) {
		source, err := client.GetGitSource(ctx, in.Name)
		if err != nil {
			return nil, apiclient.GitSourceResource{}, fmt.Errorf("get git source for app %q: %w", in.Name, err)
		}
		return nil, source, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_app_hook_runs",
		Description: "Most recent outcome of an app's pre-deploy and post-deploy hooks: exit code, success and captured output. A hook that never ran is absent.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, apiclient.AppHookRunsResource, error) {
		runs, err := client.GetAppHookRuns(ctx, in.Name)
		if err != nil {
			return nil, apiclient.AppHookRunsResource{}, fmt.Errorf("get hook runs for app %q: %w", in.Name, err)
		}
		return nil, runs, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_domain_tls_cert_status",
		Description: "Whether a domain uses an uploaded (BYO) TLS certificate instead of ACME, with upload and expiry. Never the certificate or key.",
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
