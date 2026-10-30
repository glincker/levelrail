package mcptools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
		Description: "List every OAuth sign-in provider (google, github, oidc) and its settings, with a has_client_secret boolean. The secret is never returned.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, []apiclient.OAuthProviderSettingsResource, error) {
		settings, err := client.ListOAuthProviderSettings(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("list oauth provider settings: %w", err)
		}
		return nil, settings, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_email_settings",
		Description: "Outbound email settings: backend, non-secret fields and configured providers (one primary, at most one backup). Secrets return only as *_set booleans.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, emailSettingsWithProviders, error) {
		settings, err := client.GetEmailSettings(ctx)
		if err != nil {
			return nil, emailSettingsWithProviders{}, fmt.Errorf("get email settings: %w", err)
		}
		out := emailSettingsWithProviders{EmailSettingsResource: settings, Providers: []emailProviderSummary{}}
		list, err := client.ListEmailProviders(ctx)
		var apiErr *apiclient.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
			// A control plane older than email providers has no list endpoint; the settings alone are the answer.
		case err != nil:
			return nil, emailSettingsWithProviders{}, fmt.Errorf("list email providers: %w", err)
		}
		for _, p := range list.Providers {
			out.Providers = append(out.Providers, emailProviderSummary{
				ID: p.ID, Role: p.Role, Name: p.Name, Backend: p.Backend,
				Enabled: p.Enabled == nil || *p.Enabled, CredentialSet: p.SecretSet(),
				LastOKAt: p.LastOKAt, LastErrorKind: p.LastErrorKind,
			})
		}
		return nil, out, nil
	})
}

// emailSettingsWithProviders is get_email_settings' result: the legacy settings shape plus the provider list.
type emailSettingsWithProviders struct {
	apiclient.EmailSettingsResource
	Providers []emailProviderSummary `json:"providers"`
}

// emailProviderSummary carries no credential field at all, only whether one is stored.
type emailProviderSummary struct {
	ID            string     `json:"id"`
	Role          string     `json:"role"`
	Name          string     `json:"name,omitempty"`
	Backend       string     `json:"backend"`
	Enabled       bool       `json:"enabled"`
	CredentialSet bool       `json:"credential_set"`
	LastOKAt      *time.Time `json:"last_ok_at,omitempty"`
	LastErrorKind string     `json:"last_error_kind,omitempty"`
}
