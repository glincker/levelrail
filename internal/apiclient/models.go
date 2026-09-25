package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ModelStatusResource mirrors internal/api's modelStatusResource.
type ModelStatusResource struct {
	Ready   bool   `json:"ready"`
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

// ModelResource mirrors internal/api's modelResource.
type ModelResource struct {
	Name          string              `json:"name"`
	Engine        string              `json:"engine"`
	Model         string              `json:"model"`
	NodeID        string              `json:"node_id"`
	GPUCount      int                 `json:"gpu_count"`
	GPUDeviceIDs  []string            `json:"gpu_device_ids,omitempty"`
	ContextLength int                 `json:"context_length,omitempty"`
	Quantization  string              `json:"quantization,omitempty"`
	Domain        string              `json:"domain,omitempty"`
	EndpointURL   string              `json:"endpoint_url,omitempty"`
	APIKeyPrefix  string              `json:"api_key_prefix"`
	HFTokenSet    bool                `json:"hf_token_set"`
	Status        ModelStatusResource `json:"status"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// CreateModelRequest mirrors internal/api's createModelRequest.
type CreateModelRequest struct {
	Name          string   `json:"name"`
	Engine        string   `json:"engine"`
	Model         string   `json:"model"`
	NodeID        string   `json:"node_id,omitempty"`
	GPUCount      int      `json:"gpu_count,omitempty"`
	GPUDeviceIDs  []string `json:"gpu_device_ids,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	Quantization  string   `json:"quantization,omitempty"`
	Domain        string   `json:"domain,omitempty"`
	HFToken       string   `json:"hf_token,omitempty"`
}

// CreateModelResponse is a ModelResource plus the one-time API key.
type CreateModelResponse struct {
	ModelResource
	APIKey string `json:"api_key"`
}

// ModelAPIKeyResource mirrors internal/api's modelAPIKeyResponse.
type ModelAPIKeyResource struct {
	APIKey string `json:"api_key"`
}

// GPUDeviceResource mirrors internal/api's gpuDeviceResource.
type GPUDeviceResource struct {
	Index              int    `json:"index"`
	UUID               string `json:"uuid"`
	Name               string `json:"name"`
	VRAMTotalMiB       int64  `json:"vram_total_mib"`
	VRAMUsedMiB        int64  `json:"vram_used_mib"`
	UtilizationPercent int    `json:"utilization_percent"`
}

// GPUNodeResource mirrors internal/api's gpuNodeResource.
type GPUNodeResource struct {
	NodeID           string              `json:"node_id"`
	Name             string              `json:"name"`
	IsLocal          bool                `json:"is_local"`
	Present          bool                `json:"present"`
	DriverVersion    string              `json:"driver_version,omitempty"`
	RuntimeInstalled bool                `json:"runtime_installed"`
	GPUCount         int                 `json:"gpu_count"`
	TotalVRAMMiB     int64               `json:"total_vram_mib"`
	UsedVRAMMiB      int64               `json:"used_vram_mib"`
	ModelCount       int                 `json:"model_count"`
	Hint             string              `json:"hint,omitempty"`
	Devices          []GPUDeviceResource `json:"devices"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// ListModels calls GET /api/v1/models.
func (c *Client) ListModels(ctx context.Context) ([]ModelResource, error) {
	var out []ModelResource
	err := c.do(ctx, http.MethodGet, "/api/v1/models", nil, &out)
	return out, err
}

// GetModel calls GET /api/v1/models/{name}.
func (c *Client) GetModel(ctx context.Context, name string) (ModelResource, error) {
	var out ModelResource
	err := c.do(ctx, http.MethodGet, "/api/v1/models/"+PathEscape(name), nil, &out)
	return out, err
}

// CreateModel calls POST /api/v1/models. The response carries the API key
// exactly once.
func (c *Client) CreateModel(ctx context.Context, req CreateModelRequest) (CreateModelResponse, error) {
	var out CreateModelResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/models", req, &out)
	return out, err
}

// DeleteModel calls DELETE /api/v1/models/{name}.
func (c *Client) DeleteModel(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/models/"+PathEscape(name), nil, nil)
}

// RestartModel calls POST /api/v1/models/{name}/restart.
func (c *Client) RestartModel(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/models/"+PathEscape(name)+"/restart", nil, nil)
}

// RotateModelAPIKey calls POST /api/v1/models/{name}/api-key.
func (c *Client) RotateModelAPIKey(ctx context.Context, name string) (ModelAPIKeyResource, error) {
	var out ModelAPIKeyResource
	err := c.do(ctx, http.MethodPost, "/api/v1/models/"+PathEscape(name)+"/api-key", nil, &out)
	return out, err
}

// SetModelHFToken calls PUT /api/v1/models/{name}/hf-token.
func (c *Client) SetModelHFToken(ctx context.Context, name, token string) error {
	body := map[string]string{"hf_token": token}
	return c.do(ctx, http.MethodPut, "/api/v1/models/"+PathEscape(name)+"/hf-token", body, nil)
}

// QueryModelLogs calls GET /api/v1/models/{name}/logs.
func (c *Client) QueryModelLogs(ctx context.Context, name string, from, to time.Time, q string) ([]LogEntryResource, error) {
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if q != "" {
		query.Set("q", q)
	}
	var out logsResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/models/"+PathEscape(name)+"/logs?"+query.Encode(), nil, &out)
	return out.Entries, err
}

// StreamModelLogs calls GET /api/v1/models/{name}/logs/stream.
func (c *Client) StreamModelLogs(ctx context.Context, name string, onEntry func(LogStreamEntry) error) error {
	return c.streamLogEvents(ctx, "/api/v1/models/"+PathEscape(name)+"/logs/stream", onEntry)
}

// ListGPUNodes calls GET /api/v1/gpus.
func (c *Client) ListGPUNodes(ctx context.Context) ([]GPUNodeResource, error) {
	var out []GPUNodeResource
	err := c.do(ctx, http.MethodGet, "/api/v1/gpus", nil, &out)
	return out, err
}
