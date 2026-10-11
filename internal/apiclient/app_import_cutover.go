package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// ImportCutoverAction is the one thing a readiness fix offers.
type ImportCutoverAction struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	API   string `json:"api,omitempty"`
	Value string `json:"value,omitempty"`
}

// ImportCutoverFix is the remedy for a warning or blocking check.
type ImportCutoverFix struct {
	Summary string               `json:"summary"`
	Action  *ImportCutoverAction `json:"action,omitempty"`
}

// ImportCutoverCheck is one readiness item.
type ImportCutoverCheck struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Status string            `json:"status"`
	Detail string            `json:"detail,omitempty"`
	Fix    *ImportCutoverFix `json:"fix,omitempty"`
}

// ImportCutoverRecord is one DNS record.
type ImportCutoverRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   int    `json:"ttl_seconds,omitempty"`
}

// ImportCutoverDomainPlan is what switching one domain involves.
type ImportCutoverDomainPlan struct {
	Domain   string                `json:"domain"`
	Method   string                `json:"method"`
	Current  []string              `json:"current,omitempty"`
	Provider string                `json:"provider,omitempty"`
	Zone     string                `json:"zone,omitempty"`
	Desired  *ImportCutoverRecord  `json:"desired,omitempty"`
	Replace  []ImportCutoverRecord `json:"replace,omitempty"`
	Message  string                `json:"message,omitempty"`
}

// ImportCutoverPlan is the readiness checklist for one staged app.
type ImportCutoverPlan struct {
	App        string                    `json:"app"`
	Verdict    string                    `json:"verdict"`
	Checks     []ImportCutoverCheck      `json:"checks"`
	Domains    []ImportCutoverDomainPlan `json:"domains"`
	HealthPath string                    `json:"health_path"`
	CheckedAt  string                    `json:"checked_at"`
}

// ImportCutoverDomainRun is one domain's progress inside a run.
type ImportCutoverDomainRun struct {
	Domain       string                `json:"domain"`
	Method       string                `json:"method"`
	Previous     []ImportCutoverRecord `json:"previous,omitempty"`
	Applied      *ImportCutoverRecord  `json:"applied,omitempty"`
	Manual       *ImportCutoverRecord  `json:"manual,omitempty"`
	ProbeStatus  int                   `json:"probe_status,omitempty"`
	Switched     bool                  `json:"switched"`
	Restored     bool                  `json:"restored"`
	Verified     bool                  `json:"verified"`
	VerifyDetail string                `json:"verify_detail,omitempty"`
	Message      string                `json:"message,omitempty"`
}

// ImportCutoverStep is one timed entry of a run.
type ImportCutoverStep struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Domain     string `json:"domain,omitempty"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// ImportCutoverRun is one dry run or switch.
type ImportCutoverRun struct {
	ID           string                   `json:"id"`
	App          string                   `json:"app"`
	Mode         string                   `json:"mode"`
	State        string                   `json:"state"`
	Method       string                   `json:"method,omitempty"`
	Awaiting     string                   `json:"awaiting,omitempty"`
	Domains      []ImportCutoverDomainRun `json:"domains"`
	Steps        []ImportCutoverStep      `json:"steps"`
	Plan         *ImportCutoverPlan       `json:"plan,omitempty"`
	Error        string                   `json:"error,omitempty"`
	CreatedAt    time.Time                `json:"created_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
	Rollbackable bool                     `json:"rollbackable"`
	InFlight     bool                     `json:"in_flight"`
}

// Settled reports whether the run needs nothing more from a poller.
func (r ImportCutoverRun) Settled() bool {
	switch r.State {
	case "ready", "live", "rolled_back", "failed":
		return true
	}
	return r.State == "switching" && r.Awaiting != ""
}

// ImportCutoverStart is the body of POST .../cutover/runs.
type ImportCutoverStart struct {
	Mode           string `json:"mode"`
	Confirm        string `json:"confirm,omitempty"`
	AcceptWarnings bool   `json:"accept_warnings,omitempty"`
}

func cutoverPath(session, item string) string {
	return appImportPath + "/sessions/" + url.PathEscape(session) + "/items/" + url.PathEscape(item) + "/cutover"
}

// AppImportCutoverPlan calls GET .../items/{item}/cutover/plan.
func (c *Client) AppImportCutoverPlan(ctx context.Context, session, item string) (ImportCutoverPlan, error) {
	var out ImportCutoverPlan
	err := c.do(ctx, http.MethodGet, cutoverPath(session, item)+"/plan", nil, &out)
	return out, err
}

// StartAppImportCutover calls POST .../items/{item}/cutover/runs.
func (c *Client) StartAppImportCutover(ctx context.Context, session, item string, body ImportCutoverStart) (ImportCutoverRun, error) {
	var out ImportCutoverRun
	err := c.do(ctx, http.MethodPost, cutoverPath(session, item)+"/runs", body, &out)
	return out, err
}

// AppImportCutoverRuns calls GET .../items/{item}/cutover/runs, newest first.
func (c *Client) AppImportCutoverRuns(ctx context.Context, session, item string) ([]ImportCutoverRun, error) {
	var out struct {
		Runs []ImportCutoverRun `json:"runs"`
	}
	err := c.do(ctx, http.MethodGet, cutoverPath(session, item)+"/runs", nil, &out)
	return out.Runs, err
}

// AppImportCutoverRun calls GET .../items/{item}/cutover/runs/{run}.
func (c *Client) AppImportCutoverRun(ctx context.Context, session, item, run string) (ImportCutoverRun, error) {
	var out ImportCutoverRun
	err := c.do(ctx, http.MethodGet, cutoverPath(session, item)+"/runs/"+url.PathEscape(run), nil, &out)
	return out, err
}

// RollbackAppImportCutover calls POST .../cutover/runs/{run}/rollback.
func (c *Client) RollbackAppImportCutover(ctx context.Context, session, item, run string) (ImportCutoverRun, error) {
	var out ImportCutoverRun
	err := c.do(ctx, http.MethodPost, cutoverPath(session, item)+"/runs/"+url.PathEscape(run)+"/rollback", map[string]any{}, &out)
	return out, err
}

// ConfirmAppImportCutoverDNS calls POST .../cutover/runs/{run}/confirm-dns.
func (c *Client) ConfirmAppImportCutoverDNS(ctx context.Context, session, item, run string) (ImportCutoverRun, error) {
	var out ImportCutoverRun
	err := c.do(ctx, http.MethodPost, cutoverPath(session, item)+"/runs/"+url.PathEscape(run)+"/confirm-dns", map[string]any{}, &out)
	return out, err
}
