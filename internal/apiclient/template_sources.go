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
	CommitSHA    string `json:"commit_sha,omitempty"`
	Templates    int    `json:"templates"`
}

// TemplateSourceEntry is one validated template of a source.
type TemplateSourceEntry struct {
	Source      string `json:"source"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentHash string `json:"content_hash,omitempty"`
	Review      string `json:"review,omitempty"`
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
	return getJSON[[]TemplateSource](ctx, c, templateSourcesPath)
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
	return getJSON[[]TemplateSourceEntry](ctx, c, templateSourcesPath+"/"+PathEscape(name)+"/templates")
}

// DeleteTemplateSource calls DELETE /api/v1/templates/sources/{name}.
func (c *Client) DeleteTemplateSource(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, templateSourcesPath+"/"+PathEscape(name), nil, nil)
}

// TemplateSourceEntryDetail is the masked manifest and compose of one entry, with the hash to approve.
type TemplateSourceEntryDetail struct {
	ID          string `json:"id"`
	ContentHash string `json:"content_hash"`
	Review      string `json:"review"`
	Manifest    string `json:"manifest"`
	Compose     string `json:"compose"`
}

// GetTemplateSourceEntry calls GET /api/v1/templates/sources/{name}/templates/{id}.
func (c *Client) GetTemplateSourceEntry(ctx context.Context, source, id string) (TemplateSourceEntryDetail, error) {
	return getJSON[TemplateSourceEntryDetail](ctx, c, templateSourcesPath+"/"+PathEscape(source)+"/templates/"+PathEscape(id))
}

// ApproveTemplateSourceEntry calls POST .../approve with the content hash that was reviewed.
func (c *Client) ApproveTemplateSourceEntry(ctx context.Context, source, id, hash string) error {
	body := map[string]string{"content_hash": hash}
	return c.do(ctx, http.MethodPost, templateSourcesPath+"/"+PathEscape(source)+"/templates/"+PathEscape(id)+"/approve", body, nil)
}
