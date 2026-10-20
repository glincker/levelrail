package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// RecentChange mirrors internal/changes.Change: one config, deploy or
// lifecycle change on an app. Keys are names only, never values.
type RecentChange struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Actor       string    `json:"actor"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail,omitempty"`
	Keys        []string  `json:"keys,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	LikelyCause bool      `json:"likely_cause,omitempty"`
}

// RecentChangesResource mirrors internal/changes.Result.
type RecentChangesResource struct {
	App           string         `json:"app"`
	Since         time.Time      `json:"since"`
	Until         time.Time      `json:"until"`
	WindowSeconds int64          `json:"window_seconds"`
	Changes       []RecentChange `json:"changes"`
	Total         int            `json:"total"`
	Truncated     bool           `json:"truncated,omitempty"`
}

// GetAppChanges calls GET /api/v1/apps/{name}/changes. Zero until means now;
// an empty window keeps the server default.
func (c *Client) GetAppChanges(ctx context.Context, name string, until time.Time, window string) (*RecentChangesResource, error) {
	v := url.Values{}
	if !until.IsZero() {
		v.Set("until", until.UTC().Format(time.RFC3339Nano))
	}
	if window != "" {
		v.Set("window", window)
	}
	path := "/api/v1/apps/" + PathEscape(name) + "/changes"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out RecentChangesResource
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
