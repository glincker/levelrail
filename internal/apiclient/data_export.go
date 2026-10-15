package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ExportOptions are the non-secret, kind-specific destination settings.
type ExportOptions struct {
	StorageTargetID string            `json:"storage_target_id,omitempty"`
	Prefix          string            `json:"prefix,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	IntervalSeconds int               `json:"interval_seconds,omitempty"`
	Backfill        bool              `json:"backfill,omitempty"`
	Region          string            `json:"region,omitempty"`
	Site            string            `json:"site,omitempty"`
	Dataset         string            `json:"dataset,omitempty"`
}

// ExportStatus is a destination's delivery state.
type ExportStatus struct {
	LastSuccessAt string `json:"last_success_at"`
	LastError     string `json:"last_error"`
	LastErrorAt   string `json:"last_error_at"`
	SentTotal     int64  `json:"sent_total"`
	DroppedTotal  int64  `json:"dropped_total"`
	FailuresTotal int64  `json:"failures_total"`
	QueueDepth    int64  `json:"queue_depth"`
	CircuitOpen   bool   `json:"circuit_open"`
	CursorAt      string `json:"cursor_at,omitempty"`
}

// ExportDestinationResource is one data export destination. Header values
// and signing keys are never returned.
type ExportDestinationResource struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	Endpoint    string        `json:"endpoint"`
	Enabled     bool          `json:"enabled"`
	Apps        []string      `json:"apps"`
	MinLevel    string        `json:"min_level"`
	Options     ExportOptions `json:"options"`
	HeaderNames []string      `json:"header_names"`
	HasHMACKey  bool          `json:"has_hmac_key"`
	HasAPIKey   bool          `json:"has_api_key"`
	Status      ExportStatus  `json:"status"`
	CreatedAt   string        `json:"created_at"`
	UpdatedAt   string        `json:"updated_at"`
}

// CreateExportDestinationRequest is the body of POST /api/v1/export/destinations.
type CreateExportDestinationRequest struct {
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	Endpoint string            `json:"endpoint,omitempty"`
	Enabled  *bool             `json:"enabled,omitempty"`
	Apps     []string          `json:"apps,omitempty"`
	MinLevel string            `json:"min_level,omitempty"`
	Options  ExportOptions     `json:"options"`
	Headers  map[string]string `json:"headers,omitempty"`
	HMACKey  string            `json:"hmac_key,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
}

// UpdateExportDestinationRequest is a partial update; nil fields are kept.
type UpdateExportDestinationRequest struct {
	Name     *string            `json:"name,omitempty"`
	Endpoint *string            `json:"endpoint,omitempty"`
	Enabled  *bool              `json:"enabled,omitempty"`
	Apps     *[]string          `json:"apps,omitempty"`
	MinLevel *string            `json:"min_level,omitempty"`
	Options  *ExportOptions     `json:"options,omitempty"`
	Headers  *map[string]string `json:"headers,omitempty"`
	HMACKey  *string            `json:"hmac_key,omitempty"`
}

// ExportTestResult is the answer of the destination test probe.
type ExportTestResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func exportDestinationPath(id string) string {
	return "/api/v1/export/destinations/" + PathEscape(id)
}

// ListExportDestinations calls GET /api/v1/export/destinations.
func (c *Client) ListExportDestinations(ctx context.Context) ([]ExportDestinationResource, error) {
	var out []ExportDestinationResource
	err := c.do(ctx, http.MethodGet, "/api/v1/export/destinations", nil, &out)
	return out, err
}

// GetExportDestination calls GET /api/v1/export/destinations/{id}.
func (c *Client) GetExportDestination(ctx context.Context, id string) (ExportDestinationResource, error) {
	var out ExportDestinationResource
	err := c.do(ctx, http.MethodGet, exportDestinationPath(id), nil, &out)
	return out, err
}

// CreateExportDestination calls POST /api/v1/export/destinations.
func (c *Client) CreateExportDestination(ctx context.Context, req CreateExportDestinationRequest) (ExportDestinationResource, error) {
	var out ExportDestinationResource
	err := c.do(ctx, http.MethodPost, "/api/v1/export/destinations", req, &out)
	return out, err
}

// UpdateExportDestination calls PATCH /api/v1/export/destinations/{id}.
func (c *Client) UpdateExportDestination(ctx context.Context, id string, req UpdateExportDestinationRequest) (ExportDestinationResource, error) {
	var out ExportDestinationResource
	err := c.do(ctx, http.MethodPatch, exportDestinationPath(id), req, &out)
	return out, err
}

// DeleteExportDestination calls DELETE /api/v1/export/destinations/{id}.
func (c *Client) DeleteExportDestination(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, exportDestinationPath(id), nil, nil)
}

// TestExportDestination calls POST /api/v1/export/destinations/{id}/test.
func (c *Client) TestExportDestination(ctx context.Context, id string) (ExportTestResult, error) {
	var out ExportTestResult
	err := c.do(ctx, http.MethodPost, exportDestinationPath(id)+"/test", nil, &out)
	return out, err
}

// ExportAuditLog calls GET /api/v1/audit-log/export and returns the raw
// NDJSON or CSV body.
func (c *Client) ExportAuditLog(ctx context.Context, from, to, format string) ([]byte, error) {
	q := url.Values{}
	if from != "" {
		q.Set("from", from)
	}
	if to != "" {
		q.Set("to", to)
	}
	if format != "" {
		q.Set("format", format)
	}
	return c.downloadRaw(ctx, "/api/v1/audit-log/export?"+q.Encode())
}
