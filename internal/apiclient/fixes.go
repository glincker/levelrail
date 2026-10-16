package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// FixChange is one thing a fix would do.
type FixChange struct {
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// FixAttempt is one recorded apply or re-check.
type FixAttempt struct {
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
	At      string `json:"at"`
	Actor   string `json:"actor"`
}

// Fix is a fix as GET /api/v1/fixes returns it.
type Fix struct {
	ID       string       `json:"id"`
	CheckID  string       `json:"check_id"`
	Subject  string       `json:"subject"`
	Title    string       `json:"title"`
	Why      string       `json:"why"`
	Risk     string       `json:"risk"`
	State    string       `json:"state"`
	Kind     string       `json:"kind"`
	Failing  bool         `json:"failing"`
	Detail   string       `json:"detail"`
	Reason   string       `json:"reason,omitempty"`
	Blocked  string       `json:"blocked,omitempty"`
	Command  string       `json:"command,omitempty"`
	Changes  []FixChange  `json:"changes"`
	Attempts []FixAttempt `json:"attempts"`
}

// FixResult is POST /api/v1/fixes/{id}/apply's body.
type FixResult struct {
	FixID   string `json:"fix_id"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
	View    Fix    `json:"view"`
}

// ListFixes calls GET /api/v1/fixes.
func (c *Client) ListFixes(ctx context.Context, subject string, all bool) ([]Fix, error) {
	q := url.Values{}
	if subject != "" {
		q.Set("subject", subject)
	}
	if all {
		q.Set("all", "1")
	}
	var out struct {
		Fixes []Fix `json:"fixes"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/fixes?"+q.Encode(), nil, &out)
	return out.Fixes, err
}

// PreviewFix calls POST /api/v1/fixes/{id}/preview.
func (c *Client) PreviewFix(ctx context.Context, id string) (Fix, error) {
	var out Fix
	err := c.do(ctx, http.MethodPost, "/api/v1/fixes/"+url.PathEscape(id)+"/preview", map[string]string{}, &out)
	return out, err
}

// ApplyFix calls POST /api/v1/fixes/{id}/apply.
func (c *Client) ApplyFix(ctx context.Context, id string, params map[string]string) (FixResult, error) {
	var out FixResult
	err := c.do(ctx, http.MethodPost, "/api/v1/fixes/"+url.PathEscape(id)+"/apply", map[string]any{"params": params}, &out)
	return out, err
}
