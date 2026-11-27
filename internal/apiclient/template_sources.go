package apiclient

import (
	"context"
	"net/http"
)

// TemplateSource mirrors internal/store's TemplateSource. It never carries
// key material: DeployKeyApp only names the app whose deploy key is used.
type TemplateSource struct {
	Name         string `json:"name"`
	GitURL       string `json:"git_url"`
	Ref          string `json:"ref,omitempty"`
	DeployKeyApp string `json:"deploy_key_app,omitempty"`
	Status       string `json:"status"`
	StatusDetail string `json:"status_detail,omitempty"`
	LastSync     string `json:"last_sync,omitempty"`
	CreatedAt    string `json:"created_at"`
	Templates    int    `json:"templates"`
}

// TemplateSourceEntry is one validated template of a source.
type TemplateSourceEntry struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

// TemplateSourceRequest is the body of POST /api/v1/templates/sources.
type TemplateSourceRequest struct {
	Name         string `json:"name"`
	GitURL       string `json:"git_url"`
	Ref          string `json:"ref,omitempty"`
	DeployKeyApp string `json:"deploy_key_app,omitempty"`
}

// TemplateSourceSyncResult is the answer of a sync: the source and refused templates.
type TemplateSourceSyncResult struct {
	Source  TemplateSource `json:"source"`
	Skipped []string       `json:"skipped"`
}

const templateSourcesPath = "/api/v1/templates/sources"

// ListTemplateSources calls GET /api/v1/templates/sources.
func (c *Client) ListTemplateSources(ctx context.Context) ([]TemplateSource, error) {
	var out []TemplateSource
	err := c.do(ctx, http.MethodGet, templateSourcesPath, nil, &out)
	return out, err
}

// CreateTemplateSource calls POST /api/v1/templates/sources.
func (c *Client) CreateTemplateSource(ctx context.Context, req TemplateSourceRequest) (TemplateSource, error) {
	var out TemplateSource
	err := c.do(ctx, http.MethodPost, templateSourcesPath, req, &out)
	return out, err
}

// SyncTemplateSource calls POST /api/v1/templates/sources/{name}/sync.
func (c *Client) SyncTemplateSource(ctx context.Context, name string) (TemplateSourceSyncResult, error) {
	var out TemplateSourceSyncResult
	err := c.do(ctx, http.MethodPost, templateSourcesPath+"/"+PathEscape(name)+"/sync", nil, &out)
	return out, err
}

// ListTemplateSourceEntries calls GET /api/v1/templates/sources/{name}/templates.
func (c *Client) ListTemplateSourceEntries(ctx context.Context, name string) ([]TemplateSourceEntry, error) {
	var out []TemplateSourceEntry
	err := c.do(ctx, http.MethodGet, templateSourcesPath+"/"+PathEscape(name)+"/templates", nil, &out)
	return out, err
}

// DeleteTemplateSource calls DELETE /api/v1/templates/sources/{name}.
func (c *Client) DeleteTemplateSource(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, templateSourcesPath+"/"+PathEscape(name), nil, nil)
}
