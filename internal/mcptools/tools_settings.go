package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerSettingsTools registers read-only tools for OAuth sign-in and
// outbound email settings, following registerRegistryCredentialTools and
// registerCloudflareTools' own precedent of no set/update tool for a
// credential-bearing platform-wide resource.
func registerSettingsTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_oauth_providers",
		Description: "List every OAuth sign-in provider (google, github, oidc) and its settings: enabled, client_id, allowed_email_domain, issuer_url, display_name, and a has_client_secret boolean. The client secret itself is never returned.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.OAuthProviderSettingsResource, error) {
		settings, err := client.ListOAuthProviderSettings(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list oauth provider settings: %w", err)
		}
		return nil, settings, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_email_settings",
		Description: "Outbound email settings: backend and non-secret fields. Secrets return only as *_set booleans.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.EmailSettingsResource, error) {
		settings, err := client.GetEmailSettings(ctx)
		if err != nil {
			return nil, apiclient.EmailSettingsResource{}, fmt.Errorf("get email settings: %w", err)
		}
		return nil, settings, nil
	})
}
