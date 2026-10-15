package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
)

// AdvisorFinding is one finding as GET /api/v1/resource-advisor/findings returns it.
type AdvisorFinding struct {
	resourceadvisor.Finding
	State        string `json:"state,omitempty"`
	SnoozedUntil string `json:"snoozed_until,omitempty"`
	Appliable    bool   `json:"appliable"`
	CanApply     bool   `json:"can_apply"`
}

// AdvisorFindings is GET /api/v1/resource-advisor/findings.
type AdvisorFindings struct {
	GeneratedAt string           `json:"generated_at"`
	Findings    []AdvisorFinding `json:"findings"`
}

// AdvisorSummary is GET /api/v1/resource-advisor/summary.
type AdvisorSummary struct {
	GeneratedAt string `json:"generated_at"`
	resourceadvisor.Summary
}

// AdvisorSettings is the part of a resource an advisor action changes.
type AdvisorSettings struct {
	MemoryBytes      int64 `json:"memory_bytes"`
	NanoCPUs         int64 `json:"nano_cpus"`
	SleepIdleMinutes int   `json:"sleep_idle_minutes,omitempty"`
}

// AdvisorApplyResult is POST .../findings/{id}/apply: a preview unless Applied.
type AdvisorApplyResult struct {
	FindingID       string                  `json:"finding_id"`
	Resource        resourceadvisor.Ref     `json:"resource"`
	Action          *resourceadvisor.Action `json:"action"`
	Before          AdvisorSettings         `json:"before"`
	After           AdvisorSettings         `json:"after"`
	ConfirmRequired bool                    `json:"confirm_required"`
	Applied         bool                    `json:"applied"`
	AppliedLive     bool                    `json:"applied_live"`
}

// AdvisorFindingQuery filters ListAdvisorFindings; zero fields do not filter.
type AdvisorFindingQuery struct {
	Resource      string
	Severity      string
	IncludeHidden bool
}

// ListAdvisorFindings calls GET /api/v1/resource-advisor/findings.
func (c *Client) ListAdvisorFindings(ctx context.Context, q AdvisorFindingQuery) (AdvisorFindings, error) {
	v := url.Values{}
	if q.Resource != "" {
		v.Set("resource", q.Resource)
	}
	if q.Severity != "" {
		v.Set("severity", q.Severity)
	}
	if q.IncludeHidden {
		v.Set("include", "hidden")
	}
	path := "/api/v1/resource-advisor/findings"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out AdvisorFindings
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetAdvisorSummary calls GET /api/v1/resource-advisor/summary.
func (c *Client) GetAdvisorSummary(ctx context.Context) (AdvisorSummary, error) {
	var out AdvisorSummary
	err := c.do(ctx, http.MethodGet, "/api/v1/resource-advisor/summary", nil, &out)
	return out, err
}

// ApplyAdvisorFinding previews (confirm false) or applies a finding's action.
func (c *Client) ApplyAdvisorFinding(ctx context.Context, id string, confirm bool) (AdvisorApplyResult, error) {
	var out AdvisorApplyResult
	err := c.do(ctx, http.MethodPost, "/api/v1/resource-advisor/findings/"+PathEscape(id)+"/apply", map[string]bool{"confirm": confirm}, &out)
	return out, err
}

// SetAdvisorFindingState dismisses, snoozes (for duration) or restores a finding.
func (c *Client) SetAdvisorFindingState(ctx context.Context, id, verb, duration string) (AdvisorFinding, error) {
	var body any
	if duration != "" {
		body = map[string]string{"duration": duration}
	}
	var out AdvisorFinding
	err := c.do(ctx, http.MethodPost, "/api/v1/resource-advisor/findings/"+PathEscape(id)+"/"+verb, body, &out)
	return out, err
}

// BetaFeature is one gated feature as the API describes it.
type BetaFeature struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Summary         string `json:"summary"`
	Enabled         bool   `json:"enabled"`
	EnabledBy       string `json:"enabled_by"`
	Toggleable      bool   `json:"toggleable"`
	RequiresRestart bool   `json:"requires_restart"`
	EnvHint         string `json:"env_hint"`
	DocsPath        string `json:"docs_path"`
	Maturity        string `json:"maturity"`
	Locked          bool   `json:"locked"`
}

// ListBetaFeatures calls GET /api/v1/settings/beta-features.
func (c *Client) ListBetaFeatures(ctx context.Context) ([]BetaFeature, error) {
	var out []BetaFeature
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/beta-features", nil, &out)
	return out, err
}

// SetBetaFeature calls PUT /api/v1/settings/beta-features/{feature}.
func (c *Client) SetBetaFeature(ctx context.Context, feature string, enabled bool) (BetaFeature, error) {
	var out BetaFeature
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/beta-features/"+PathEscape(feature), map[string]bool{"enabled": enabled}, &out)
	return out, err
}
