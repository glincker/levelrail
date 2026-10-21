package apiclient

import (
	"context"
	"net/http"
	"time"
)

// IssueSilenceResource mirrors internal/store's Silence plus its read-time expiry.
type IssueSilenceResource struct {
	ID        string     `json:"id"`
	IssueKey  string     `json:"issue_key"`
	Scope     string     `json:"scope,omitempty"`
	Until     *time.Time `json:"until,omitempty"`
	Severity  string     `json:"severity,omitempty"`
	CreatedBy string     `json:"created_by"`
	Reason    string     `json:"reason,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	Expired   bool       `json:"expired"`
}

// CreateIssueSilenceRequest is the body of POST /api/v1/silences.
type CreateIssueSilenceRequest struct {
	IssueKey string `json:"issue_key"`
	Scope    string `json:"scope,omitempty"`
	Duration string `json:"duration"`
	Reason   string `json:"reason,omitempty"`
	Severity string `json:"severity,omitempty"`
	Confirm  bool   `json:"confirm,omitempty"`
}

// ListIssueSilences calls GET /api/v1/silences.
func (c *Client) ListIssueSilences(ctx context.Context) ([]IssueSilenceResource, error) {
	var out struct {
		Silences []IssueSilenceResource `json:"silences"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/silences", nil, &out)
	return out.Silences, err
}

// CreateIssueSilence calls POST /api/v1/silences.
func (c *Client) CreateIssueSilence(ctx context.Context, req CreateIssueSilenceRequest) (IssueSilenceResource, error) {
	var out IssueSilenceResource
	err := c.do(ctx, http.MethodPost, "/api/v1/silences", req, &out)
	return out, err
}

// DeleteIssueSilence calls DELETE /api/v1/silences/{id}.
func (c *Client) DeleteIssueSilence(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/silences/"+PathEscape(id), nil, nil)
}
