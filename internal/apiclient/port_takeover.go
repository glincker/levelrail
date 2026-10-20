package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

const takeoverBase = "/api/v1/migration/takeover"

// TakeoverFixAction is the one primary thing a fix offers.
type TakeoverFixAction struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	API   string `json:"api,omitempty"`
	Value string `json:"value,omitempty"`
}

// TakeoverFix is the remedy for a warning or blocking check.
type TakeoverFix struct {
	Summary string             `json:"summary"`
	Action  *TakeoverFixAction `json:"action,omitempty"`
}

// TakeoverCheck is one preflight item.
type TakeoverCheck struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Status string       `json:"status"`
	Detail string       `json:"detail,omitempty"`
	Items  []string     `json:"items,omitempty"`
	Fix    *TakeoverFix `json:"fix,omitempty"`
}

// TakeoverPlanStep is one step a run would take.
type TakeoverPlanStep struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Reversible bool   `json:"reversible"`
}

// TakeoverDomain is one domain the takeover has to keep serving.
type TakeoverDomain struct {
	Domain  string `json:"domain"`
	App     string `json:"app"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail,omitempty"`
	DNS     string `json:"dns"`
	Cert    string `json:"certificate"`
}

// TakeoverPlan is the preflight result plus the exact steps.
type TakeoverPlan struct {
	Verdict       string             `json:"verdict"`
	Checks        []TakeoverCheck    `json:"checks"`
	Steps         []TakeoverPlanStep `json:"steps"`
	Domains       []TakeoverDomain   `json:"domains"`
	Rollback      []string           `json:"rollback"`
	WindowSeconds int                `json:"window_seconds"`
	TargetHTTP    string             `json:"target_http"`
	TargetHTTPS   string             `json:"target_https"`
	CurrentHTTP   string             `json:"current_http"`
	CurrentHTTPS  string             `json:"current_https"`
	ProxyKind     string             `json:"proxy_kind,omitempty"`
	ProxyNames    []string           `json:"proxy_containers"`
	CheckedAt     string             `json:"checked_at"`
}

// TakeoverStep is one timed entry of a run.
type TakeoverStep struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	Detail     string    `json:"detail,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

// TakeoverDomainCheck is the outcome of verifying one domain over HTTPS.
type TakeoverDomainCheck struct {
	Domain    string `json:"domain"`
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	Delivered bool   `json:"delivered"`
	Detail    string `json:"detail,omitempty"`
}

// TakeoverRun is one takeover and, when undone, its rollback.
type TakeoverRun struct {
	ID            string                `json:"id"`
	State         string                `json:"state"`
	Actor         string                `json:"actor,omitempty"`
	TargetHTTP    string                `json:"target_http"`
	TargetHTTPS   string                `json:"target_https"`
	Steps         []TakeoverStep        `json:"steps"`
	Verify        []TakeoverDomainCheck `json:"verify"`
	Error         string                `json:"error,omitempty"`
	WindowSeconds int                   `json:"window_seconds"`
	DeadlineAt    time.Time             `json:"deadline_at"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Rollbackable  bool                  `json:"rollbackable"`
	InFlight      bool                  `json:"in_flight"`
}

// Settled reports whether the run needs nothing more from a poller.
func (r TakeoverRun) Settled() bool {
	switch r.State {
	case "done", "rolled_back", "failed":
		return true
	}
	return false
}

// TakeoverStatus is GET takeover/plan: the plan and the latest run.
type TakeoverStatus struct {
	Plan TakeoverPlan `json:"plan"`
	Run  *TakeoverRun `json:"run"`
}

// TakeoverStart is the body of POST takeover/runs.
type TakeoverStart struct {
	Confirm         bool `json:"confirm"`
	AcceptUnmanaged bool `json:"accept_unmanaged"`
}

// TakeoverStartResult is the answer to POST takeover/runs.
type TakeoverStartResult struct {
	DryRun  bool          `json:"dry_run"`
	Created bool          `json:"created"`
	Plan    *TakeoverPlan `json:"plan,omitempty"`
	Run     *TakeoverRun  `json:"run,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// TakeoverStatus calls GET /api/v1/migration/takeover/plan.
func (c *Client) TakeoverStatus(ctx context.Context, acceptUnmanaged bool) (TakeoverStatus, error) {
	path := takeoverBase + "/plan"
	if acceptUnmanaged {
		path += "?accept_unmanaged=true"
	}
	var out TakeoverStatus
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// StartTakeover calls POST /api/v1/migration/takeover/runs.
func (c *Client) StartTakeover(ctx context.Context, body TakeoverStart) (TakeoverStartResult, error) {
	var out TakeoverStartResult
	err := c.do(ctx, http.MethodPost, takeoverBase+"/runs", body, &out)
	return out, err
}

// TakeoverRuns calls GET /api/v1/migration/takeover/runs, newest first.
func (c *Client) TakeoverRuns(ctx context.Context) ([]TakeoverRun, error) {
	var out struct {
		Runs []TakeoverRun `json:"runs"`
	}
	err := c.do(ctx, http.MethodGet, takeoverBase+"/runs", nil, &out)
	return out.Runs, err
}

// TakeoverRun calls GET /api/v1/migration/takeover/runs/{run}.
func (c *Client) TakeoverRun(ctx context.Context, id string) (TakeoverRun, error) {
	var out TakeoverRun
	err := c.do(ctx, http.MethodGet, takeoverBase+"/runs/"+url.PathEscape(id), nil, &out)
	return out, err
}

// RollbackTakeover calls POST /api/v1/migration/takeover/runs/{run}/rollback.
func (c *Client) RollbackTakeover(ctx context.Context, id, reason string) (TakeoverStartResult, error) {
	var out TakeoverStartResult
	err := c.do(ctx, http.MethodPost, takeoverBase+"/runs/"+url.PathEscape(id)+"/rollback", map[string]string{"reason": reason}, &out)
	return out, err
}
