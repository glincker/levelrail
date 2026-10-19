package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// DashboardResource mirrors a custom dashboard; Doc is the portable document, kept raw so export round trips exactly.
type DashboardResource struct {
	ID        string          `json:"id"`
	Owner     string          `json:"owner"`
	Scope     string          `json:"scope"`
	ProjectID string          `json:"project_id,omitempty"`
	IsDefault bool            `json:"is_default"`
	Name      string          `json:"name"`
	Editable  bool            `json:"editable"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Doc       json.RawMessage `json:"doc"`
}

// DashboardWrite is the body of a dashboard create or update.
type DashboardWrite struct {
	Scope     string          `json:"scope,omitempty"`
	ProjectID string          `json:"project_id,omitempty"`
	IsDefault bool            `json:"is_default,omitempty"`
	Doc       json.RawMessage `json:"doc"`
}

// DashboardTemplate is a built-in starter dashboard.
type DashboardTemplate struct {
	Key         string          `json:"key"`
	NeedsTarget string          `json:"needs_target,omitempty"`
	Doc         json.RawMessage `json:"doc"`
}

// ListDashboards calls GET /api/v1/dashboards, optionally narrowed to a project.
func (c *Client) ListDashboards(ctx context.Context, project string) ([]DashboardResource, error) {
	path := "/api/v1/dashboards"
	if project != "" {
		path += "?project=" + url.QueryEscape(project)
	}
	var out []DashboardResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetDashboard calls GET /api/v1/dashboards/{id}.
func (c *Client) GetDashboard(ctx context.Context, id string) (DashboardResource, error) {
	var out DashboardResource
	err := c.do(ctx, http.MethodGet, "/api/v1/dashboards/"+PathEscape(id), nil, &out)
	return out, err
}

// CreateDashboard calls POST /api/v1/dashboards.
func (c *Client) CreateDashboard(ctx context.Context, in DashboardWrite) (DashboardResource, error) {
	var out DashboardResource
	err := c.do(ctx, http.MethodPost, "/api/v1/dashboards", in, &out)
	return out, err
}

// ExportDashboard calls GET /api/v1/dashboards/{id}/export and returns the portable document.
func (c *Client) ExportDashboard(ctx context.Context, id string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, "/api/v1/dashboards/"+PathEscape(id)+"/export", nil, &out)
	return out, err
}

// ImportDashboard calls POST /api/v1/dashboards/import with an exported document.
func (c *Client) ImportDashboard(ctx context.Context, doc json.RawMessage, scope, project string) (DashboardResource, error) {
	q := url.Values{}
	if scope != "" {
		q.Set("scope", scope)
	}
	if project != "" {
		q.Set("project", project)
	}
	path := "/api/v1/dashboards/import"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out DashboardResource
	err := c.do(ctx, http.MethodPost, path, doc, &out)
	return out, err
}

// DeleteDashboard calls DELETE /api/v1/dashboards/{id}.
func (c *Client) DeleteDashboard(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/dashboards/"+PathEscape(id), nil, nil)
}

// DashboardTemplates calls GET /api/v1/dashboards/templates.
func (c *Client) DashboardTemplates(ctx context.Context) ([]DashboardTemplate, error) {
	var out []DashboardTemplate
	err := c.do(ctx, http.MethodGet, "/api/v1/dashboards/templates", nil, &out)
	return out, err
}
