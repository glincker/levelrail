package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PipelineParameter mirrors a declared run parameter.
type PipelineParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Default     string   `json:"default,omitempty"`
	Choices     []string `json:"choices,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
}

// PipelineArtifact is one archive a job stored.
type PipelineArtifact struct {
	ID        string    `json:"id"`
	Job       string    `json:"job"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PipelineWebhook is a pipeline's inbound webhook trigger. Token and Secret
// are set only on the response that minted them.
type PipelineWebhook struct {
	Configured bool       `json:"configured"`
	Enabled    bool       `json:"enabled,omitempty"`
	HMAC       bool       `json:"hmac,omitempty"`
	Path       string     `json:"path,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	RotatedAt  *time.Time `json:"rotated_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Token      string     `json:"token,omitempty"`
	Secret     string     `json:"secret,omitempty"`
}

// PipelineWebhookRequest is the body of PUT .../webhook.
type PipelineWebhookRequest struct {
	HMAC    *bool `json:"hmac,omitempty"`
	Enabled *bool `json:"enabled,omitempty"`
}

// PipelineWebhookDelivery is one request a trigger received.
type PipelineWebhookDelivery struct {
	ID             string    `json:"id"`
	ReceivedAt     time.Time `json:"received_at"`
	StatusCode     int       `json:"status_code"`
	Outcome        string    `json:"outcome"`
	Reason         string    `json:"reason,omitempty"`
	RunID          string    `json:"run_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	ContentType    string    `json:"content_type,omitempty"`
	BodyBytes      int       `json:"body_bytes"`
	Replayable     bool      `json:"replayable"`
}

// ListPipelineRunArtifacts calls GET .../pipeline-runs/{id}/artifacts.
func (c *Client) ListPipelineRunArtifacts(ctx context.Context, app, runID string) ([]PipelineArtifact, error) {
	var out []PipelineArtifact
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipeline-runs", runID, "artifacts"), nil, &out)
}

// DownloadPipelineArtifact calls GET .../artifacts/{id}/download and returns the archive.
func (c *Client) DownloadPipelineArtifact(ctx context.Context, app, runID, artifactID string) ([]byte, error) {
	return c.downloadRaw(ctx, pipelinePath(app, "pipeline-runs", runID, "artifacts", artifactID, "download"))
}

// GetPipelineWebhook calls GET .../pipelines/{name}/webhook.
func (c *Client) GetPipelineWebhook(ctx context.Context, app, name string) (PipelineWebhook, error) {
	var out PipelineWebhook
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipelines", name, "webhook"), nil, &out)
}

// SetPipelineWebhook calls PUT .../pipelines/{name}/webhook.
func (c *Client) SetPipelineWebhook(ctx context.Context, app, name string, req PipelineWebhookRequest) (PipelineWebhook, error) {
	var out PipelineWebhook
	return out, c.do(ctx, http.MethodPut, pipelinePath(app, "pipelines", name, "webhook"), req, &out)
}

// RotatePipelineWebhook calls POST .../pipelines/{name}/webhook/rotate.
func (c *Client) RotatePipelineWebhook(ctx context.Context, app, name string) (PipelineWebhook, error) {
	var out PipelineWebhook
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipelines", name, "webhook", "rotate"), nil, &out)
}

// DeletePipelineWebhook calls DELETE .../pipelines/{name}/webhook.
func (c *Client) DeletePipelineWebhook(ctx context.Context, app, name string) error {
	return c.do(ctx, http.MethodDelete, pipelinePath(app, "pipelines", name, "webhook"), nil, nil)
}

// ListPipelineWebhookDeliveries calls GET .../webhook/deliveries.
func (c *Client) ListPipelineWebhookDeliveries(ctx context.Context, app, name string) ([]PipelineWebhookDelivery, error) {
	var out []PipelineWebhookDelivery
	return out, c.do(ctx, http.MethodGet, pipelinePath(app, "pipelines", name, "webhook", "deliveries"), nil, &out)
}

// ReplayPipelineWebhookDelivery calls POST .../webhook/deliveries/{id}/replay.
func (c *Client) ReplayPipelineWebhookDelivery(ctx context.Context, app, name, deliveryID string) (PipelineRunResource, error) {
	var out PipelineRunResource
	return out, c.do(ctx, http.MethodPost, pipelinePath(app, "pipelines", name, "webhook", "deliveries", deliveryID, "replay"), nil, &out)
}
