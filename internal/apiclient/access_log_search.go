package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// AccessSearchRecord mirrors internal/api's accessSearchRow. The edge never
// records query strings, and client IPs are not part of any read response.
type AccessSearchRecord struct {
	Time       time.Time `json:"time"`
	App        string    `json:"app,omitempty"`
	Host       string    `json:"host"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	DurationMs float64   `json:"duration_ms"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	RequestID  string    `json:"request_id"`
	TraceID    string    `json:"trace_id,omitempty"`
}

// AccessSearchResource mirrors internal/api's accessSearchResponse.
type AccessSearchResource struct {
	Records    []AccessSearchRecord `json:"records"`
	NextCursor string               `json:"next_cursor,omitempty"`
	Truncated  bool                 `json:"truncated"`
	From       time.Time            `json:"from"`
	To         time.Time            `json:"to"`
	MaxRows    int                  `json:"max_rows"`
}

// SearchAccessLog calls GET /api/v1/logs/access with the given filters
// (app, host, method, path, q, status, status_min, status_max, min_ms, since, from, to, limit, cursor).
func (c *Client) SearchAccessLog(ctx context.Context, filters url.Values) (AccessSearchResource, error) {
	var out AccessSearchResource
	err := c.do(ctx, http.MethodGet, "/api/v1/logs/access?"+filters.Encode(), nil, &out)
	return out, err
}
