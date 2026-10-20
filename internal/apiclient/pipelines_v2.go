package apiclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

// DownloadPipelineArtifact calls GET .../artifacts/{id}/download and streams
// the archive into w, hashing as it goes. It returns the byte count and the
// sha256 it computed; a mismatch with the server's X-Content-SHA256 header
// or the Content-Length is an error.
func (c *Client) DownloadPipelineArtifact(ctx context.Context, app, runID, artifactID string, w io.Writer) (int64, string, error) {
	url := c.baseURL + pipelinePath(app, "pipeline-runs", runID, "artifacts", artifactID, "download")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // baseURL is the operator-supplied API target
	if err != nil {
		return 0, "", fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	start := time.Now()
	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		TraceRequest(http.MethodGet, url, "", time.Since(start), err)
		return 0, "", fmt.Errorf("request GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	TraceRequest(http.MethodGet, url, resp.Status, time.Since(start), nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return 0, "", &APIError{StatusCode: resp.StatusCode, Message: ExtractErrorMessage(data), RetryAfter: retryAfterHeader(resp.Header)}
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	if err != nil {
		return n, "", fmt.Errorf("read response body: %w", err)
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return n, "", fmt.Errorf("download was cut short: got %d of %d bytes", n, resp.ContentLength)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if want := resp.Header.Get("X-Content-SHA256"); want != "" && want != sum {
		return n, sum, fmt.Errorf("sha256 mismatch: server sent %s, computed %s", want, sum)
	}
	return n, sum, nil
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
