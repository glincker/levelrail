package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// TimelineRef points at the record behind a timeline item.
type TimelineRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// TimelineItem mirrors internal/api's timelineItem.
type TimelineItem struct {
	ID     string       `json:"id"`
	At     string       `json:"at"`
	Kind   string       `json:"kind"`
	Status string       `json:"status"`
	Actor  string       `json:"actor"`
	Title  string       `json:"title"`
	Detail string       `json:"detail,omitempty"`
	Ref    *TimelineRef `json:"ref,omitempty"`
}

// TimelineResponse mirrors internal/api's timelineResponse.
type TimelineResponse struct {
	Items      []TimelineItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// PendingChange mirrors internal/api's pendingChange.
type PendingChange struct {
	Kind  string   `json:"kind"`
	Keys  []string `json:"keys,omitempty"`
	Since string   `json:"since"`
}

// PendingChanges mirrors internal/api's pendingChangesResource.
type PendingChanges struct {
	Pending     bool            `json:"pending"`
	Changes     []PendingChange `json:"changes"`
	ApplyAction string          `json:"apply_action"`
}

// ApplyPendingResult mirrors internal/api's applyPendingResult.
type ApplyPendingResult struct {
	AttemptID string `json:"attempt_id,omitempty"`
}

// GetAppTimeline calls GET /api/v1/apps/{name}/timeline. limit 0 keeps the
// server default; before is the previous page's next_cursor.
func (c *Client) GetAppTimeline(ctx context.Context, name string, limit int, before string) (TimelineResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if before != "" {
		q.Set("before", before)
	}
	path := "/api/v1/apps/" + PathEscape(name) + "/timeline"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out TimelineResponse
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetPendingChanges calls GET /api/v1/apps/{name}/pending-changes.
func (c *Client) GetPendingChanges(ctx context.Context, name string) (PendingChanges, error) {
	var out PendingChanges
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/pending-changes", nil, &out)
	return out, err
}

// ApplyPendingChanges calls POST /api/v1/apps/{name}/apply-pending.
func (c *Client) ApplyPendingChanges(ctx context.Context, name string) (ApplyPendingResult, error) {
	var out ApplyPendingResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/apply-pending", nil, &out)
	return out, err
}

// DeleteSecret calls DELETE /api/v1/apps/{name}/secrets/{key}: removes the
// value and undeclares the key. force overrides a locked key.
func (c *Client) DeleteSecret(ctx context.Context, name, key string, force bool) error {
	path := "/api/v1/apps/" + PathEscape(name) + "/secrets/" + PathEscape(key)
	if force {
		path += "?force=true"
	}
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
