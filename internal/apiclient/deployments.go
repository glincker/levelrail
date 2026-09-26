package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DeploymentSteps is a live deployment's step tally.
type DeploymentSteps struct {
	Done        int    `json:"done"`
	Running     int    `json:"running"`
	Failed      int    `json:"failed"`
	FailingStep string `json:"failing_step,omitempty"`
}

// DeploymentResource is one row of GET /api/v1/deployments.
type DeploymentResource struct {
	ID              string           `json:"id"`
	App             string           `json:"app"`
	Status          string           `json:"status"`
	Trigger         string           `json:"trigger"`
	Environment     string           `json:"environment"`
	Image           string           `json:"image"`
	ImageRef        string           `json:"image_ref"`
	ImageDigest     string           `json:"image_digest"`
	DigestReason    string           `json:"digest_reason"`
	RolloutState    string           `json:"rollout_state"`
	CommitSHA       string           `json:"commit_sha"`
	Branch          string           `json:"branch"`
	CommitMessage   string           `json:"commit_message"`
	Author          string           `json:"author"`
	PRNumber        *int             `json:"pr_number"`
	StartedAt       time.Time        `json:"started_at"`
	FinishedAt      *time.Time       `json:"finished_at"`
	DurationMS      *int64           `json:"duration_ms"`
	Steps           *DeploymentSteps `json:"steps"`
	ErrorSummary    *string          `json:"error_summary"`
	ReasonCode      string           `json:"reason_code"`
	Reason          string           `json:"reason"`
	RollbackOf      *string          `json:"rollback_of"`
	RolledBackBy    *string          `json:"rolled_back_by"`
	SupersededBy    *string          `json:"superseded_by"`
	IsLive          bool             `json:"is_live"`
	ApprovalID      *string          `json:"approval_id"`
	PreviewImageURL *string          `json:"preview_image_url"`
}

// DeploymentList is the GET /api/v1/deployments response.
type DeploymentList struct {
	Items      []DeploymentResource `json:"items"`
	NextCursor string               `json:"next_cursor"`
}

// DeploymentDay is one sparkline bucket of the deployments summary.
type DeploymentDay struct {
	Date   string `json:"date"`
	Total  int    `json:"total"`
	Failed int    `json:"failed"`
}

// DeploymentSummary is the GET /api/v1/deployments/summary response.
type DeploymentSummary struct {
	Window         string         `json:"window"`
	Counts         map[string]int `json:"counts"`
	InProgress     int            `json:"in_progress"`
	NeedsAttention int            `json:"needs_attention"`
	FailureRate24h *float64       `json:"failure_rate_24h"`
	Duration       struct {
		MedianMS *int64 `json:"median_ms"`
		P95MS    *int64 `json:"p95_ms"`
		Samples  int    `json:"samples"`
	} `json:"duration"`
	PerDay []DeploymentDay `json:"per_day"`
}

// DeploymentEvent is one message of GET /api/v1/deployments/stream.
type DeploymentEvent struct {
	Type string `json:"type"`
	Step *struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"step,omitempty"`
	Deployment DeploymentResource `json:"deployment"`
}

// DeploymentListOptions are the GET /api/v1/deployments filters.
type DeploymentListOptions struct {
	Statuses    []string
	App         string
	Branch      string
	Triggers    []string
	Environment string
	Since       string
	Until       string
	Query       string
	Live        bool
	PR          int
	Limit       int
	Cursor      string
}

// Values renders the options as a query string, omitting unset filters.
func (o DeploymentListOptions) Values() url.Values {
	v := url.Values{}
	set := func(k, s string) {
		if s != "" {
			v.Set(k, s)
		}
	}
	set("status", strings.Join(o.Statuses, ","))
	set("trigger", strings.Join(o.Triggers, ","))
	set("app", o.App)
	set("branch", o.Branch)
	set("environment", o.Environment)
	set("since", o.Since)
	set("until", o.Until)
	set("q", o.Query)
	set("cursor", o.Cursor)
	if o.Live {
		v.Set("live", "true")
	}
	if o.PR > 0 {
		v.Set("pr", strconv.Itoa(o.PR))
	}
	if o.Limit > 0 {
		v.Set("limit", strconv.Itoa(o.Limit))
	}
	return v
}

// ListDeployments calls GET /api/v1/deployments.
func (c *Client) ListDeployments(ctx context.Context, opts DeploymentListOptions) (DeploymentList, error) {
	path := "/api/v1/deployments"
	if q := opts.Values().Encode(); q != "" {
		path += "?" + q
	}
	var out DeploymentList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetDeploymentsSummary calls GET /api/v1/deployments/summary; window is a
// duration such as 24h or 7d, empty for the server default.
func (c *Client) GetDeploymentsSummary(ctx context.Context, window string) (DeploymentSummary, error) {
	path := "/api/v1/deployments/summary"
	if window != "" {
		path += "?window=" + url.QueryEscape(window)
	}
	var out DeploymentSummary
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// StreamDeployments calls GET /api/v1/deployments/stream and invokes onEvent
// for each change until onEvent errors or ctx is canceled.
func (c *Client) StreamDeployments(ctx context.Context, onEvent func(DeploymentEvent) error) error {
	return streamSSE(ctx, c, "/api/v1/deployments/stream", onEvent)
}
