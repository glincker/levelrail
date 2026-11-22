package apiclient

import (
	"context"
	"encoding/json"
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
	return getJSON[Connection](ctx, c, "/api/v1/connections/"+PathEscape(ref))
}

// ConnectionCatalog calls GET /api/v1/connections/catalog.
func (c *Client) ConnectionCatalog(ctx context.Context) ([]ConnectionCatalogEntry, error) {
	var out struct {
		Catalog []ConnectionCatalogEntry `json:"catalog"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/connections/catalog", nil, &out)
	return out.Catalog, err
}

// ConnectionBinding mirrors one binding row; Origin is "binding" for stored rows.
type ConnectionBinding struct {
	ID         string `json:"id,omitempty"`
	Ref        string `json:"ref"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Role       string `json:"role"`
	Rules      string `json:"rules,omitempty"`
	Origin     string `json:"origin"`
}

// ResolvedConnectionBinding is an effective row for an app, own or inherited from its org.
type ResolvedConnectionBinding struct {
	ConnectionBinding
	InheritedFrom string `json:"inherited_from,omitempty"`
	Overridden    bool   `json:"overridden,omitempty"`
}

// AppConnectionBindings is the GET /api/v1/apps/{name}/bindings body.
type AppConnectionBindings struct {
	Bindings []ConnectionBinding         `json:"bindings"`
	Resolved []ResolvedConnectionBinding `json:"resolved"`
}

// BindConnectionRequest is the body of a bind call; Rules is raw JSON for the role.
type BindConnectionRequest struct {
	Ref   string          `json:"ref"`
	Role  string          `json:"role"`
	Rules json.RawMessage `json:"rules,omitempty"`
}

func appBindingsPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/bindings"
}

func orgBindingsPath(org string) string {
	return "/api/v1/organizations/" + PathEscape(org) + "/bindings"
}

// BindAppConnection calls POST /api/v1/apps/{name}/bindings.
func (c *Client) BindAppConnection(ctx context.Context, app string, req BindConnectionRequest) (ConnectionBinding, error) {
	var out ConnectionBinding
	err := c.do(ctx, http.MethodPost, appBindingsPath(app), req, &out)
	return out, err
}

// UnbindAppConnection calls DELETE /api/v1/apps/{name}/bindings/{id}.
func (c *Client) UnbindAppConnection(ctx context.Context, app, id string) error {
	return c.del(ctx, appBindingsPath(app)+"/"+PathEscape(id))
}

// ListAppConnectionBindings calls GET /api/v1/apps/{name}/bindings.
func (c *Client) ListAppConnectionBindings(ctx context.Context, app string) (AppConnectionBindings, error) {
	return getJSON[AppConnectionBindings](ctx, c, appBindingsPath(app))
}

// BindOrgConnection calls POST /api/v1/organizations/{id}/bindings.
func (c *Client) BindOrgConnection(ctx context.Context, org string, req BindConnectionRequest) (ConnectionBinding, error) {
	var out ConnectionBinding
	err := c.do(ctx, http.MethodPost, orgBindingsPath(org), req, &out)
	return out, err
}

// UnbindOrgConnection calls DELETE /api/v1/organizations/{id}/bindings/{bid}.
func (c *Client) UnbindOrgConnection(ctx context.Context, org, id string) error {
	return c.del(ctx, orgBindingsPath(org)+"/"+PathEscape(id))
}

// ListOrgConnectionBindings calls GET /api/v1/organizations/{id}/bindings.
func (c *Client) ListOrgConnectionBindings(ctx context.Context, org string) ([]ConnectionBinding, error) {
	var out struct {
		Bindings []ConnectionBinding `json:"bindings"`
	}
	err := c.do(ctx, http.MethodGet, orgBindingsPath(org), nil, &out)
	return out.Bindings, err
}
