package apiclient

import (
	"context"
	"net/http"
)

// AuditChainBreak locates the first row that failed chain verification.
type AuditChainBreak struct {
	Seq    int64  `json:"seq"`
	ID     string `json:"id,omitempty"`
	Reason string `json:"reason"`
}

// AuditChainReport mirrors GET /api/v1/audit-log/verify.
type AuditChainReport struct {
	OK          bool             `json:"ok"`
	Chained     int64            `json:"chained"`
	Unchained   int64            `json:"unchained"`
	HeadSeq     int64            `json:"head_seq"`
	HeadHash    string           `json:"head_hash"`
	FirstBroken *AuditChainBreak `json:"first_broken,omitempty"`
}

// AuditSinkConfig is the non-secret, per-kind configuration of a sink.
type AuditSinkConfig struct {
	Address   string `json:"address,omitempty"`
	TLS       bool   `json:"tls,omitempty"`
	URL       string `json:"url,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Region    string `json:"region,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	PathStyle bool   `json:"path_style,omitempty"`
}

// AuditSink mirrors GET /api/v1/audit-sinks items. SigningSecret is set only
// on create and rotate responses.
type AuditSink struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	Kind                string          `json:"kind"`
	Enabled             bool            `json:"enabled"`
	Config              AuditSinkConfig `json:"config"`
	CursorSeq           int64           `json:"cursor_seq"`
	ConsecutiveFailures int             `json:"consecutive_failures"`
	NextAttemptAt       string          `json:"next_attempt_at,omitempty"`
	LastError           string          `json:"last_error,omitempty"`
	LastDeliveredAt     string          `json:"last_delivered_at,omitempty"`
	CreatedAt           string          `json:"created_at"`
	UpdatedAt           string          `json:"updated_at"`
	SigningSecret       string          `json:"signing_secret,omitempty"`
}

// AuditSinkDelivery is one batch attempt in a sink's history.
type AuditSinkDelivery struct {
	ID        string `json:"id"`
	SinkID    string `json:"sink_id"`
	FromSeq   int64  `json:"from_seq"`
	ToSeq     int64  `json:"to_seq"`
	Entries   int    `json:"entries"`
	State     string `json:"state"`
	Attempt   int    `json:"attempt"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt string `json:"created_at"`
}

// CreateAuditSinkRequest is the POST /api/v1/audit-sinks body.
type CreateAuditSinkRequest struct {
	Name            string          `json:"name"`
	Kind            string          `json:"kind"`
	Enabled         *bool           `json:"enabled,omitempty"`
	Config          AuditSinkConfig `json:"config"`
	AccessKeyID     string          `json:"access_key_id,omitempty"`
	SecretAccessKey string          `json:"secret_access_key,omitempty"`
	Backfill        bool            `json:"backfill,omitempty"`
}

// UpdateAuditSinkRequest is the PATCH body; nil fields stay unchanged.
type UpdateAuditSinkRequest struct {
	Name            *string          `json:"name,omitempty"`
	Enabled         *bool            `json:"enabled,omitempty"`
	Config          *AuditSinkConfig `json:"config,omitempty"`
	AccessKeyID     *string          `json:"access_key_id,omitempty"`
	SecretAccessKey *string          `json:"secret_access_key,omitempty"`
}

// AuditSinkTestResult is the answer of the test probe.
type AuditSinkTestResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func auditSinkPath(id string) string { return "/api/v1/audit-sinks/" + PathEscape(id) }

// VerifyAuditChain calls GET /api/v1/audit-log/verify.
func (c *Client) VerifyAuditChain(ctx context.Context) (AuditChainReport, error) {
	var out AuditChainReport
	err := c.do(ctx, http.MethodGet, "/api/v1/audit-log/verify", nil, &out)
	return out, err
}

// ListAuditSinks calls GET /api/v1/audit-sinks.
func (c *Client) ListAuditSinks(ctx context.Context) ([]AuditSink, error) {
	var out []AuditSink
	err := c.do(ctx, http.MethodGet, "/api/v1/audit-sinks", nil, &out)
	return out, err
}

// CreateAuditSink calls POST /api/v1/audit-sinks.
func (c *Client) CreateAuditSink(ctx context.Context, req CreateAuditSinkRequest) (AuditSink, error) {
	var out AuditSink
	err := c.do(ctx, http.MethodPost, "/api/v1/audit-sinks", req, &out)
	return out, err
}

// UpdateAuditSink calls PATCH /api/v1/audit-sinks/{id}.
func (c *Client) UpdateAuditSink(ctx context.Context, id string, req UpdateAuditSinkRequest) (AuditSink, error) {
	var out AuditSink
	err := c.do(ctx, http.MethodPatch, auditSinkPath(id), req, &out)
	return out, err
}

// DeleteAuditSink calls DELETE /api/v1/audit-sinks/{id}.
func (c *Client) DeleteAuditSink(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, auditSinkPath(id), nil, nil)
}

// TestAuditSink calls POST /api/v1/audit-sinks/{id}/test.
func (c *Client) TestAuditSink(ctx context.Context, id string) (AuditSinkTestResult, error) {
	var out AuditSinkTestResult
	err := c.do(ctx, http.MethodPost, auditSinkPath(id)+"/test", nil, &out)
	return out, err
}

// RotateAuditSinkSecret calls POST /api/v1/audit-sinks/{id}/rotate-secret.
func (c *Client) RotateAuditSinkSecret(ctx context.Context, id string) (AuditSink, error) {
	var out AuditSink
	err := c.do(ctx, http.MethodPost, auditSinkPath(id)+"/rotate-secret", nil, &out)
	return out, err
}

// ListAuditSinkDeliveries calls GET /api/v1/audit-sinks/{id}/deliveries.
func (c *Client) ListAuditSinkDeliveries(ctx context.Context, id string) ([]AuditSinkDelivery, error) {
	var out []AuditSinkDelivery
	err := c.do(ctx, http.MethodGet, auditSinkPath(id)+"/deliveries", nil, &out)
	return out, err
}
