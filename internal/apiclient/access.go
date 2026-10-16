package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AccessRecord mirrors internal/ingress's AccessRecord.
type AccessRecord struct {
	Time      time.Time `json:"time"`
	Host      string    `json:"host"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Millis    float64   `json:"duration_ms"`
	BytesIn   int64     `json:"bytes_in"`
	BytesOut  int64     `json:"bytes_out"`
	RequestID string    `json:"request_id"`
	TraceID   string    `json:"trace_id,omitempty"`
	Upstream  bool      `json:"upstream_error,omitempty"`
}

// AccessOptions are the filters of ListAccess.
type AccessOptions struct {
	MinMillis float64
	MinStatus int
	Path      string
	Since     string
	Limit     int
}

// ListAccess calls GET /api/v1/apps/{name}/access.
func (c *Client) ListAccess(ctx context.Context, app string, o AccessOptions) ([]AccessRecord, error) {
	q := url.Values{}
	if o.MinMillis > 0 {
		q.Set("min_ms", strconv.FormatFloat(o.MinMillis, 'f', -1, 64))
	}
	if o.MinStatus > 0 {
		q.Set("min_status", strconv.Itoa(o.MinStatus))
	}
	if o.Path != "" {
		q.Set("path", o.Path)
	}
	if o.Since != "" {
		q.Set("since", o.Since)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/access"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []AccessRecord
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
