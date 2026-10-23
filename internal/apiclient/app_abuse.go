package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AbusePolicy is an app's ingress abuse defense policy.
type AbusePolicy struct {
	Mode             string   `json:"mode"`
	RPS              float64  `json:"rps,omitempty"`
	Burst            int      `json:"burst,omitempty"`
	DenyCIDRs        []string `json:"deny_cidrs,omitempty"`
	DenyUserAgents   []string `json:"deny_user_agents,omitempty"`
	RequireUserAgent bool     `json:"require_user_agent,omitempty"`
}

// AbuseCondition is the status the ingress reported for the policy.
type AbuseCondition struct {
	Status  string    `json:"status"`
	Reason  string    `json:"reason"`
	Message string    `json:"message"`
	Since   time.Time `json:"since,omitempty"`
}

// AbuseReasonCount is one reason's would-block and blocked counts.
type AbuseReasonCount struct {
	Reason     string  `json:"reason"`
	WouldBlock float64 `json:"would_block"`
	Blocked    float64 `json:"blocked"`
}

// AbuseCounters is the defense's tally over a recent window.
type AbuseCounters struct {
	Allowed    float64            `json:"allowed"`
	WouldBlock float64            `json:"would_block"`
	Blocked    float64            `json:"blocked"`
	FailOpen   float64            `json:"fail_open"`
	ByReason   []AbuseReasonCount `json:"by_reason"`
}

// AbuseSuggestion is a starting rate derived from the app's recent traffic.
type AbuseSuggestion struct {
	PeakRPS     float64 `json:"peak_rps"`
	RPS         float64 `json:"rps"`
	Burst       int     `json:"burst"`
	FromTraffic bool    `json:"from_traffic"`
}

// AbuseLimits are the instance caps a policy is validated against.
type AbuseLimits struct {
	MaxRPS         float64 `json:"max_rps"`
	MaxBurst       int     `json:"max_burst"`
	MaxDenyEntries int     `json:"max_deny_entries"`
	MaxAgentEntry  int     `json:"max_agent_entries"`
	DryRunDefault  int     `json:"dry_run_default_minutes"`
	DryRunMax      int     `json:"dry_run_max_minutes"`
}

// AppAbuse is GET/PUT /api/v1/apps/{name}/abuse.
type AppAbuse struct {
	App                   string          `json:"app"`
	Configured            bool            `json:"configured"`
	Policy                AbusePolicy     `json:"policy"`
	UpdatedAt             string          `json:"updated_at,omitempty"`
	UpdatedBy             string          `json:"updated_by,omitempty"`
	Condition             AbuseCondition  `json:"condition"`
	Suggestion            AbuseSuggestion `json:"suggestion"`
	Counters              AbuseCounters   `json:"counters"`
	CountersWindowMinutes int             `json:"counters_window_minutes"`
	Limits                AbuseLimits     `json:"limits"`
}

// AbuseClientImpact is one client's share of a dry run.
type AbuseClientImpact struct {
	Client     string `json:"client"`
	Requests   int    `json:"requests"`
	WouldBlock int    `json:"would_block"`
	TopReason  string `json:"top_reason"`
}

// AbuseReasonTotal pairs a reason with a request count.
type AbuseReasonTotal struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// AbuseDryRun is the answer to POST /api/v1/apps/{name}/abuse/dry-run. Nothing was blocked.
type AbuseDryRun struct {
	Requests       int                 `json:"requests"`
	WouldBlock     int                 `json:"would_block"`
	Percent        float64             `json:"percent"`
	ByReason       []AbuseReasonTotal  `json:"by_reason"`
	TopClients     []AbuseClientImpact `json:"top_clients"`
	From           time.Time           `json:"from"`
	To             time.Time           `json:"to"`
	WindowMinutes  float64             `json:"window_minutes"`
	CoveredMinutes float64             `json:"covered_minutes"`
	Note           string              `json:"note"`
}

// AbuseDryRunRequest is the dry-run body; a nil Policy replays the saved one.
type AbuseDryRunRequest struct {
	Policy  *AbusePolicy `json:"policy,omitempty"`
	Minutes int          `json:"minutes,omitempty"`
}

func appAbusePath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/abuse"
}

// GetAppAbuse calls GET /api/v1/apps/{name}/abuse.
func (c *Client) GetAppAbuse(ctx context.Context, app string) (AppAbuse, error) {
	var out AppAbuse
	err := c.do(ctx, http.MethodGet, appAbusePath(app), nil, &out)
	return out, err
}

// SetAppAbuse calls PUT /api/v1/apps/{name}/abuse; it replaces the whole policy.
func (c *Client) SetAppAbuse(ctx context.Context, app string, p AbusePolicy) (AppAbuse, error) {
	var out AppAbuse
	err := c.do(ctx, http.MethodPut, appAbusePath(app), p, &out)
	return out, err
}

// DryRunAppAbuse calls POST /api/v1/apps/{name}/abuse/dry-run.
func (c *Client) DryRunAppAbuse(ctx context.Context, app string, req AbuseDryRunRequest) (AbuseDryRun, error) {
	var out AbuseDryRun
	err := c.do(ctx, http.MethodPost, appAbusePath(app)+"/dry-run", req, &out)
	return out, err
}
