package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ModelKeyResource mirrors internal/api's modelKeyResource. It never
// carries key material, only the prefix.
type ModelKeyResource struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Status      string     `json:"status"`
	RPM         int        `json:"rpm"`
	TPM         int        `json:"tpm"`
	MaxParallel int        `json:"max_parallel"`
	AllowPaths  []string   `json:"allow_paths"`
	AllowModels []string   `json:"allow_models"`
	ReplacedBy  string     `json:"replaced_by,omitempty"`
	InFlight    int        `json:"in_flight"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// CreatedModelKeyResource is a key plus its one-time plaintext.
type CreatedModelKeyResource struct {
	ModelKeyResource
	APIKey string `json:"api_key"`
}

// CreateModelKeyRequest is the body of POST /api/v1/models/{name}/keys.
type CreateModelKeyRequest struct {
	Name        string     `json:"name"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RPM         int        `json:"rpm,omitempty"`
	TPM         int        `json:"tpm,omitempty"`
	MaxParallel int        `json:"max_parallel,omitempty"`
	AllowPaths  []string   `json:"allow_paths,omitempty"`
	AllowModels []string   `json:"allow_models,omitempty"`
}

// ModelUsageTotals mirrors models.UsageTotals.
type ModelUsageTotals struct {
	Requests      int64 `json:"requests"`
	Status2xx     int64 `json:"status_2xx"`
	Status4xx     int64 `json:"status_4xx"`
	Status5xx     int64 `json:"status_5xx"`
	RateLimited   int64 `json:"rate_limited"`
	UsageRequests int64 `json:"usage_requests"`
	InputTokens   int64 `json:"input_tokens"`
	OutputTokens  int64 `json:"output_tokens"`
	BytesOut      int64 `json:"bytes_out"`
	AvgDurationMs int64 `json:"avg_duration_ms"`
	AvgTTFTMs     int64 `json:"avg_ttft_ms"`
}

// ModelUsagePoint is one hour of usage.
type ModelUsagePoint struct {
	Hour time.Time `json:"hour"`
	ModelUsageTotals
}

// ModelKeyUsage is one key's usage in the window.
type ModelKeyUsage struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name"`
	KeyPrefix string `json:"key_prefix"`
	Status    string `json:"status"`
	InFlight  int    `json:"in_flight"`
	ModelUsageTotals
}

// ModelUsageReport mirrors models.UsageReport.
type ModelUsageReport struct {
	Model    string            `json:"model"`
	From     time.Time         `json:"from"`
	To       time.Time         `json:"to"`
	Totals   ModelUsageTotals  `json:"totals"`
	Series   []ModelUsagePoint `json:"series"`
	Keys     []ModelKeyUsage   `json:"keys"`
	InFlight int               `json:"in_flight"`
	Note     string            `json:"note"`
}

func modelKeysPath(name string) string { return "/api/v1/models/" + PathEscape(name) + "/keys" }

// ListModelKeys calls GET /api/v1/models/{name}/keys.
func (c *Client) ListModelKeys(ctx context.Context, name string) ([]ModelKeyResource, error) {
	var out []ModelKeyResource
	err := c.do(ctx, http.MethodGet, modelKeysPath(name), nil, &out)
	return out, err
}

// CreateModelKey calls POST /api/v1/models/{name}/keys.
func (c *Client) CreateModelKey(ctx context.Context, name string, req CreateModelKeyRequest) (CreatedModelKeyResource, error) {
	var out CreatedModelKeyResource
	err := c.do(ctx, http.MethodPost, modelKeysPath(name), req, &out)
	return out, err
}

// RevokeModelKey calls DELETE /api/v1/models/{name}/keys/{id}.
func (c *Client) RevokeModelKey(ctx context.Context, name, id string) error {
	return c.do(ctx, http.MethodDelete, modelKeysPath(name)+"/"+PathEscape(id), nil, nil)
}

// RotateModelKey calls POST /api/v1/models/{name}/keys/{id}/rotate. A nil
// grace uses the server default; zero retires the old key at once.
func (c *Client) RotateModelKey(ctx context.Context, name, id string, grace *time.Duration) (CreatedModelKeyResource, error) {
	body := map[string]int{}
	if grace != nil {
		body["grace_seconds"] = int(grace.Seconds())
	}
	var out CreatedModelKeyResource
	err := c.do(ctx, http.MethodPost, modelKeysPath(name)+"/"+PathEscape(id)+"/rotate", body, &out)
	return out, err
}

// GetModelUsage calls GET /api/v1/models/{name}/usage.
func (c *Client) GetModelUsage(ctx context.Context, name string, since time.Duration) (ModelUsageReport, error) {
	q := url.Values{}
	if since > 0 {
		q.Set("since", since.String())
	}
	var out ModelUsageReport
	err := c.do(ctx, http.MethodGet, "/api/v1/models/"+PathEscape(name)+"/usage?"+q.Encode(), nil, &out)
	return out, err
}
