package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AlertLogRule is the match config of a log_pattern or log_level_rate rule.
type AlertLogRule struct {
	Query           string   `json:"query,omitempty"`
	Regex           string   `json:"regex,omitempty"`
	Fields          []string `json:"fields,omitempty"`
	Level           string   `json:"level,omitempty"`
	Count           int      `json:"count,omitempty"`
	PerMinute       float64  `json:"per_minute,omitempty"`
	WindowSeconds   int      `json:"window_seconds,omitempty"`
	CooldownSeconds int      `json:"cooldown_seconds,omitempty"`
}

// LogQuerySpec is the search a saved log query reproduces.
type LogQuerySpec struct {
	Q         string     `json:"q,omitempty"`
	Level     string     `json:"level,omitempty"`
	Stream    string     `json:"stream,omitempty"`
	Fields    []string   `json:"fields,omitempty"`
	Container string     `json:"container,omitempty"`
	Range     string     `json:"range,omitempty"`
	From      *time.Time `json:"from,omitempty"`
	To        *time.Time `json:"to,omitempty"`
}

// SavedLogQueryRequest creates or replaces a saved log query.
type SavedLogQueryRequest struct {
	Name       string       `json:"name"`
	App        string       `json:"app,omitempty"`
	Spec       LogQuerySpec `json:"spec"`
	Visibility string       `json:"visibility,omitempty"`
	Pinned     bool         `json:"pinned,omitempty"`
}

// SavedLogQuery mirrors internal/api's savedLogQueryResource.
type SavedLogQuery struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	App        string       `json:"app,omitempty"`
	Spec       LogQuerySpec `json:"spec"`
	Visibility string       `json:"visibility"`
	Pinned     bool         `json:"pinned"`
	Owner      string       `json:"owner,omitempty"`
	Mine       bool         `json:"mine"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

// LogSignature is one cluster of similar error lines.
type LogSignature struct {
	ID        string    `json:"id"`
	Template  string    `json:"template"`
	Level     string    `json:"level"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Buckets   []int     `json:"buckets"`
	Sample    string    `json:"sample"`
	New       bool      `json:"new,omitempty"`
}

// LogSignatures mirrors internal/api's logSignaturesResponse.
type LogSignatures struct {
	From        time.Time      `json:"from"`
	To          time.Time      `json:"to"`
	Scanned     int            `json:"scanned"`
	MaxRows     int            `json:"max_rows"`
	Truncated   bool           `json:"truncated"`
	Total       int            `json:"total"`
	NewCount    int            `json:"new_count"`
	HasBaseline bool           `json:"has_baseline"`
	Signatures  []LogSignature `json:"signatures"`
}

// LogSignatureQuery selects the window and shape of a signature report.
type LogSignatureQuery struct {
	From     time.Time
	To       time.Time
	Level    string
	Text     string
	Limit    int
	Baseline bool
}

// GetLogSignatures calls GET /api/v1/apps/{name}/logs/signatures.
func (c *Client) GetLogSignatures(ctx context.Context, app string, s LogSignatureQuery) (LogSignatures, error) {
	q := url.Values{}
	q.Set("from", s.From.UTC().Format(time.RFC3339))
	q.Set("to", s.To.UTC().Format(time.RFC3339))
	if s.Level != "" {
		q.Set("level", s.Level)
	}
	if s.Text != "" {
		q.Set("q", s.Text)
	}
	if s.Limit > 0 {
		q.Set("limit", strconv.Itoa(s.Limit))
	}
	if s.Baseline {
		q.Set("baseline", "true")
	}
	var out LogSignatures
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/logs/signatures?"+q.Encode(), nil, &out)
	return out, err
}

// ListSavedLogQueries calls GET /api/v1/logs/queries.
func (c *Client) ListSavedLogQueries(ctx context.Context, app string) ([]SavedLogQuery, error) {
	path := "/api/v1/logs/queries"
	if app != "" {
		path += "?app=" + url.QueryEscape(app)
	}
	var out struct {
		Queries []SavedLogQuery `json:"queries"`
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out.Queries, err
}

// CreateSavedLogQuery calls POST /api/v1/logs/queries.
func (c *Client) CreateSavedLogQuery(ctx context.Context, req SavedLogQueryRequest) (SavedLogQuery, error) {
	var out SavedLogQuery
	err := c.do(ctx, http.MethodPost, "/api/v1/logs/queries", req, &out)
	return out, err
}

// DeleteSavedLogQuery calls DELETE /api/v1/logs/queries/{id}.
func (c *Client) DeleteSavedLogQuery(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/logs/queries/"+PathEscape(id), nil, nil)
}
