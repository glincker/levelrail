package main

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_oauth_providers",
		Description: "List every OAuth sign-in provider (google, github, oidc) and its settings: enabled, client_id, allowed_email_domain, issuer_url, display_name, and a has_client_secret boolean. The client secret itself is never returned. Read-only; does not enable, disable, or edit a provider.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.OAuthProviderSettingsResource, error) {
		settings, err := client.ListOAuthProviderSettings(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list oauth provider settings: %w", err)
		}
		return nil, settings, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_email_settings",
		Description: "Get the control plane's outbound email settings: backend (smtp/ses/disabled) and its non-secret fields (host, port, username, from address). SMTP password and SES secret access key are never returned, only smtp_password_set/ses_secret_access_key_set booleans reporting whether one is stored. Read-only; does not configure or test outbound email.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, apiclient.EmailSettingsResource, error) {
		settings, err := client.GetEmailSettings(ctx)
		if err != nil {
			return nil, apiclient.EmailSettingsResource{}, fmt.Errorf("get email settings: %w", err)
		}
		return nil, settings, nil
	})
}
