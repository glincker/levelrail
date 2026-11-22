package apiclient

import (
	"context"
	"net/http"
)

// MCPClient mirrors internal/api's mcpClientResource.
type MCPClient struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       []string `json:"scopes"`
	CreatedAt    string   `json:"created_at"`
}

// MCPClientCreate is the POST body for a pre-registered remote MCP OAuth client.
type MCPClientCreate struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	Scopes       []string `json:"scopes,omitempty"`
}

// CreateMCPClient calls POST /api/v1/mcp/oauth/clients.
func (c *Client) CreateMCPClient(ctx context.Context, in MCPClientCreate) (MCPClient, error) {
	var out MCPClient
	err := c.do(ctx, http.MethodPost, "/api/v1/mcp/oauth/clients", in, &out)
	return out, err
}

// ListMCPClients calls GET /api/v1/mcp/oauth/clients.
func (c *Client) ListMCPClients(ctx context.Context) ([]MCPClient, error) {
	return getJSON[[]MCPClient](ctx, c, "/api/v1/mcp/oauth/clients")
}

// DeleteMCPClient calls DELETE /api/v1/mcp/oauth/clients/{id}, which also revokes its tokens.
func (c *Client) DeleteMCPClient(ctx context.Context, id string) error {
	return c.del(ctx, "/api/v1/mcp/oauth/clients/"+PathEscape(id))
}
