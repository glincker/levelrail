package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ChecklistAction mirrors internal/golive's Action.
type ChecklistAction struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	FixID   string `json:"fix_id,omitempty"`
	API     string `json:"api,omitempty"`
	Href    string `json:"href,omitempty"`
	Value   string `json:"value,omitempty"`
	Blocked string `json:"blocked,omitempty"`
}

// ChecklistEvidence is one fact behind a step.
type ChecklistEvidence struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ChecklistFirewallFix mirrors golive.FixPreview.
type ChecklistFirewallFix struct {
	FixID      string   `json:"fix_id"`
	Kind       string   `json:"kind"`
	Commands   []string `json:"commands"`
	Undo       []string `json:"undo"`
	Persistent bool     `json:"persistent"`
	Note       string   `json:"note,omitempty"`
	Refusal    string   `json:"refusal,omitempty"`
}

// ChecklistProvider is the cloud provider's firewall guidance.
type ChecklistProvider struct {
	Name    string   `json:"name"`
	DocsURL string   `json:"docs_url"`
	Steps   []string `json:"steps"`
	Note    string   `json:"note,omitempty"`
}

// ChecklistStep mirrors golive.Step.
type ChecklistStep struct {
	ID          string                `json:"id"`
	Title       string                `json:"title"`
	Status      string                `json:"status"`
	Summary     string                `json:"summary"`
	Evidence    []ChecklistEvidence   `json:"evidence"`
	Primary     *ChecklistAction      `json:"primary,omitempty"`
	NextCheckAt *time.Time            `json:"next_check_at,omitempty"`
	FirewallFix *ChecklistFirewallFix `json:"firewall_fix,omitempty"`
	Provider    *ChecklistProvider    `json:"provider,omitempty"`
	Scope       string                `json:"scope,omitempty"`
}

// ChecklistEvent is one timeline entry.
type ChecklistEvent struct {
	ID     string    `json:"id"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Title  string    `json:"title"`
	Detail string    `json:"detail,omitempty"`
	Failed bool      `json:"failed,omitempty"`
	Future bool      `json:"future,omitempty"`
}

// GoLiveChecklist mirrors golive.Checklist.
type GoLiveChecklist struct {
	App       string           `json:"app"`
	Domain    string           `json:"domain"`
	State     string           `json:"state"`
	Live      bool             `json:"live"`
	Next      string           `json:"next,omitempty"`
	URL       string           `json:"url,omitempty"`
	Steps     []ChecklistStep  `json:"steps"`
	Timeline  []ChecklistEvent `json:"timeline"`
	CheckedAt time.Time        `json:"checked_at"`
}

// GoLiveChecklist calls GET /api/v1/apps/{app}/go-live/checklist.
func (c *Client) GoLiveChecklist(ctx context.Context, app, domain string) (GoLiveChecklist, error) {
	path := "/api/v1/apps/" + url.PathEscape(app) + "/go-live/checklist"
	if domain != "" {
		path += "?domain=" + url.QueryEscape(domain)
	}
	var out GoLiveChecklist
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
