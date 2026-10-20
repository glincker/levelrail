package mcptools

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const samlScimFeatureID = "saml-scim"

type ssoConnectionBrief struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	Configured     bool     `json:"configured"`
	IDPEntityID    string   `json:"idp_entity_id,omitempty"`
	AllowedDomains []string `json:"allowed_domains"`
	JIT            bool     `json:"jit_provisioning"`
	SyncRoles      bool     `json:"sync_roles"`
	CertExpires    string   `json:"cert_expires,omitempty"`
}

type ssoConnectionsOutput struct {
	Maturity    string               `json:"maturity,omitempty"`
	Enabled     bool                 `json:"feature_enabled"`
	EnforceSSO  bool                 `json:"enforce_sso"`
	Connections []ssoConnectionBrief `json:"connections"`
}

type scimTokenBrief struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	Revoked    bool    `json:"revoked"`
}

type scimStatusOutput struct {
	BaseURL     string           `json:"base_url"`
	DefaultRole string           `json:"default_role_id"`
	Tokens      []scimTokenBrief `json:"tokens"`
}

type ssoGroupMappingsOutput struct {
	Mappings []apiclient.SSOGroupMapping `json:"mappings"`
}

// registerIdentityTools lists SSO and SCIM state without secrets; every
// route behind it needs the root ability.
func registerIdentityTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "list_sso_connections",
		Description: "SAML connections with maturity, status and cert expiry, no secrets. Root only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ssoConnectionsOutput, error) {
		conns, err := client.ListSAMLConnections(ctx)
		if err != nil {
			return nil, ssoConnectionsOutput{}, fmt.Errorf("list sso connections: %w", err)
		}
		out := ssoConnectionsOutput{Connections: make([]ssoConnectionBrief, 0, len(conns))}
		if s, err := client.GetSSOSettings(ctx); err == nil {
			out.EnforceSSO = s.EnforceSSO
		}
		if feats, err := client.ListBetaFeatures(ctx); err == nil {
			for _, f := range feats {
				if f.ID == samlScimFeatureID {
					out.Maturity, out.Enabled = f.Maturity, f.Enabled
				}
			}
		}
		for _, c := range conns {
			b := ssoConnectionBrief{ID: c.ID, Name: c.Name, Enabled: c.Enabled, Configured: c.Configured, IDPEntityID: c.IDPEntityID, AllowedDomains: c.AllowedDomains, JIT: c.JITProvisioning, SyncRoles: c.SyncRoles}
			for _, cert := range c.Certificates {
				if b.CertExpires == "" || cert.NotAfter < b.CertExpires {
					b.CertExpires = cert.NotAfter
				}
			}
			out.Connections = append(out.Connections, b)
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "get_scim_status",
		Description: "SCIM base URL, default role and token metadata, never a token value. Root only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, scimStatusOutput, error) {
		s, err := client.GetSSOSettings(ctx)
		if err != nil {
			return nil, scimStatusOutput{}, fmt.Errorf("get scim status: %w", err)
		}
		toks, err := client.ListSCIMTokens(ctx)
		if err != nil {
			return nil, scimStatusOutput{}, fmt.Errorf("get scim status: %w", err)
		}
		out := scimStatusOutput{BaseURL: s.SCIMBaseURL, DefaultRole: s.SCIMDefaultRole, Tokens: make([]scimTokenBrief, 0, len(toks))}
		for _, t := range toks {
			out.Tokens = append(out.Tokens, scimTokenBrief{ID: t.ID, Name: t.Name, Prefix: t.Prefix, LastUsedAt: t.LastUsedAt, Revoked: t.RevokedAt != nil})
		}
		return nil, out, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "list_sso_group_mappings",
		Description: "Which directory group maps to which role, by priority. Root only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ssoGroupMappingsOutput, error) {
		m, err := client.ListSSOGroupMappings(ctx)
		if err != nil {
			return nil, ssoGroupMappingsOutput{}, fmt.Errorf("list sso group mappings: %w", err)
		}
		if m.Mappings == nil {
			m.Mappings = []apiclient.SSOGroupMapping{}
		}
		return nil, ssoGroupMappingsOutput{Mappings: m.Mappings}, nil
	})
}
