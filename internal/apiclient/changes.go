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
	// Revisions are the recorded config versions, newest first.
	Revisions []ConfigRevision `json:"revisions,omitempty"`
}

// ConfigFieldChange is one line of a config diff. Secret values never appear:
// a masked env value is "[masked]" and a secret is "secret:NAME".
type ConfigFieldChange struct {
	Kind  string `json:"kind"`
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// ConfigRevision mirrors internal/api configRevision.
type ConfigRevision struct {
	N        int                 `json:"n"`
	At       time.Time           `json:"at"`
	Actor    string              `json:"actor"`
	Kinds    []string            `json:"kinds"`
	Changes  []ConfigFieldChange `json:"changes"`
	RevertOf int                 `json:"revert_of,omitempty"`
}

// RevertResult mirrors internal/api revertResponse.
type RevertResult struct {
	App          string              `json:"app"`
	RevertedTo   int                 `json:"reverted_to"`
	Revision     int                 `json:"revision,omitempty"`
	DryRun       bool                `json:"dry_run,omitempty"`
	Changes      []ConfigFieldChange `json:"changes"`
	Skipped      []string            `json:"skipped,omitempty"`
	PendingApply bool                `json:"pending_apply"`
}

// RevertApp calls POST /api/v1/apps/{name}/revert. dryRun only previews.
func (c *Client) RevertApp(ctx context.Context, name string, to int, dryRun bool) (*RevertResult, error) {
	var out RevertResult
	body := map[string]any{"to": to, "dry_run": dryRun}
	if err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/revert", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
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
