package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

// AppTypeSummary is one row of GET /api/v1/app-types.
type AppTypeSummary struct {
	Name          string     `json:"name"`
	LatestVersion int        `json:"latest_version"`
	UpdatedBy     string     `json:"updated_by,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

// AppTypePublished is the body of a publish call.
type AppTypePublished struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	Digest  string `json:"digest"`
}

// AppTypeCheckRequest carries a version (0 means newest) and an app spec.
type AppTypeCheckRequest struct {
	Version int            `json:"version,omitempty"`
	Spec    map[string]any `json:"spec"`
}

// AppTypeCheckResult is the body of POST /api/v1/app-types/{name}/check.
type AppTypeCheckResult struct {
	Name       string            `json:"name"`
	Version    int               `json:"version"`
	Violations []PolicyViolation `json:"violations"`
	WouldFail  bool              `json:"would_fail"`
}

// ListAppTypes calls GET /api/v1/app-types.
func (c *Client) ListAppTypes(ctx context.Context) ([]AppTypeSummary, error) {
	var out []AppTypeSummary
	err := c.do(ctx, http.MethodGet, "/api/v1/app-types", nil, &out)
	return out, err
}

// SaveAppType calls POST /api/v1/app-types with a raw type document.
func (c *Client) SaveAppType(ctx context.Context, doc json.RawMessage) error {
	return c.do(ctx, http.MethodPost, "/api/v1/app-types", doc, nil)
}

// PublishAppType calls POST /api/v1/app-types/{name}/publish.
func (c *Client) PublishAppType(ctx context.Context, name string) (AppTypePublished, error) {
	var out AppTypePublished
	err := c.do(ctx, http.MethodPost, "/api/v1/app-types/"+url.PathEscape(name)+"/publish", struct{}{}, &out)
	return out, err
}

// CheckAppType calls POST /api/v1/app-types/{name}/check.
func (c *Client) CheckAppType(ctx context.Context, name string, req AppTypeCheckRequest) (AppTypeCheckResult, error) {
	var out AppTypeCheckResult
	err := c.do(ctx, http.MethodPost, "/api/v1/app-types/"+url.PathEscape(name)+"/check", req, &out)
	return out, err
}
