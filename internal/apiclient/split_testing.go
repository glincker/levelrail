package apiclient

import (
	"context"
	"net/http"
	"time"
)

// SplitVariant mirrors one variant of internal/api's split test resource.
type SplitVariant struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Kind     string  `json:"kind"`
	Ref      string  `json:"ref,omitempty"`
	Weight   int     `json:"weight"`
	Status   string  `json:"status"`
	Host     string  `json:"host,omitempty"`
	Requests uint64  `json:"requests"`
	Share    float64 `json:"share"`
	QALink   string  `json:"qa_link,omitempty"`
}

// SplitBucket is one hour of per-variant request counts.
type SplitBucket struct {
	Bucket time.Time         `json:"bucket"`
	Counts map[string]uint64 `json:"counts"`
}

// SplitTest mirrors internal/api's split test resource.
type SplitTest struct {
	Active        bool           `json:"active"`
	Supported     bool           `json:"supported"`
	Unsupported   string         `json:"unsupported_reason,omitempty"`
	TestID        string         `json:"test_id,omitempty"`
	Status        string         `json:"status,omitempty"`
	CookieName    string         `json:"cookie_name,omitempty"`
	CookieTTLSecs int            `json:"cookie_ttl_secs,omitempty"`
	OverrideParam string         `json:"override_param,omitempty"`
	CreatedAt     *time.Time     `json:"created_at,omitempty"`
	EndedAt       *time.Time     `json:"ended_at,omitempty"`
	EndReason     string         `json:"end_reason,omitempty"`
	TotalRequests uint64         `json:"total_requests"`
	Variants      []SplitVariant `json:"variants"`
	Hourly        []SplitBucket  `json:"hourly,omitempty"`
}

// SplitVariantInput is one non-production arm in a start request.
type SplitVariantInput struct {
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Weight int    `json:"weight"`
}

// SplitStartRequest is the body of POST /api/v1/apps/{name}/split. The
// production arm gets the leftover weight when it is not listed.
type SplitStartRequest struct {
	Variants   []SplitVariantInput `json:"variants"`
	CookieName string              `json:"cookie_name,omitempty"`
	CookieTTL  string              `json:"cookie_ttl,omitempty"`
}

func splitPath(appName string) string { return "/api/v1/apps/" + PathEscape(appName) + "/split" }

// GetSplit calls GET /api/v1/apps/{name}/split.
func (c *Client) GetSplit(ctx context.Context, appName string) (SplitTest, error) {
	var out SplitTest
	err := c.do(ctx, http.MethodGet, splitPath(appName), nil, &out)
	return out, err
}

// StartSplit calls POST /api/v1/apps/{name}/split.
func (c *Client) StartSplit(ctx context.Context, appName string, req SplitStartRequest) (SplitTest, error) {
	var out SplitTest
	err := c.do(ctx, http.MethodPost, splitPath(appName), req, &out)
	return out, err
}

// SetSplitWeights calls PUT /api/v1/apps/{name}/split; keys are variant IDs or labels.
func (c *Client) SetSplitWeights(ctx context.Context, appName string, weights map[string]int) (SplitTest, error) {
	var out SplitTest
	err := c.do(ctx, http.MethodPut, splitPath(appName), map[string]map[string]int{"weights": weights}, &out)
	return out, err
}

// StopSplit calls DELETE /api/v1/apps/{name}/split.
func (c *Client) StopSplit(ctx context.Context, appName string) (SplitTest, error) {
	var out SplitTest
	err := c.do(ctx, http.MethodDelete, splitPath(appName), nil, &out)
	return out, err
}

// PromoteSplit calls POST /api/v1/apps/{name}/split/promote.
func (c *Client) PromoteSplit(ctx context.Context, appName, variant string) (SplitTest, error) {
	var out SplitTest
	err := c.do(ctx, http.MethodPost, splitPath(appName)+"/promote", map[string]string{"variant": variant}, &out)
	return out, err
}
