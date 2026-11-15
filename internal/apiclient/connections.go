package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Connection mirrors one row of the integrations hub read model.
type Connection struct {
	Ref          string     `json:"ref"`
	Kind         string     `json:"kind"`
	Provider     string     `json:"provider"`
	Label        string     `json:"label"`
	Account      string     `json:"account,omitempty"`
	Host         string     `json:"host,omitempty"`
	ScopeType    string     `json:"scope_type"`
	ScopeID      string     `json:"scope_id,omitempty"`
	AuthMethod   string     `json:"auth_method"`
	Health       string     `json:"health"`
	HealthDetail string     `json:"health_detail,omitempty"`
	CheckedAt    *time.Time `json:"checked_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	ExpiringSoon bool       `json:"expiring_soon"`
	RotateAfter  *time.Time `json:"rotate_after,omitempty"`
	Capabilities []string   `json:"capabilities"`
	BoundCount   int        `json:"bound_count"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
	IsDefault    bool       `json:"is_default,omitempty"`
}

// ConnectionCatalogEntry is one provider the control plane can connect.
type ConnectionCatalogEntry struct {
	ID             string   `json:"id"`
	Category       string   `json:"category"`
	Name           string   `json:"name"`
	ConnectMethods []string `json:"connect_methods"`
	CreateEndpoint string   `json:"create_endpoint,omitempty"`
}

// ConnectionFilter narrows a listing; empty fields match everything.
type ConnectionFilter struct {
	Kind   string
	Scope  string
	Health string
}

// ListConnections calls GET /api/v1/connections.
func (c *Client) ListConnections(ctx context.Context, f ConnectionFilter) ([]Connection, error) {
	q := url.Values{}
	for k, v := range map[string]string{"kind": f.Kind, "scope": f.Scope, "health": f.Health} {
		if v != "" {
			q.Set(k, v)
		}
	}
	path := "/api/v1/connections"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out struct {
		Connections []Connection `json:"connections"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Connections, err
}

// GetConnection calls GET /api/v1/connections/{ref}.
func (c *Client) GetConnection(ctx context.Context, ref string) (Connection, error) {
	var out Connection
	err := c.do(ctx, http.MethodGet, "/api/v1/connections/"+PathEscape(ref), nil, &out)
	return out, err
}

// ConnectionCatalog calls GET /api/v1/connections/catalog.
func (c *Client) ConnectionCatalog(ctx context.Context) ([]ConnectionCatalogEntry, error) {
	var out struct {
		Catalog []ConnectionCatalogEntry `json:"catalog"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/connections/catalog", nil, &out)
	return out.Catalog, err
}
