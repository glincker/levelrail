package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/migrateverify"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
)

// MigrationVerifyRequest is the source credential plus the probe settings
// for POST /api/v1/migrations/verify. Recheck reuses the last request.
type MigrationVerifyRequest struct {
	MigrationSourceRequest
	Recheck             bool   `json:"recheck,omitempty"`
	SkipProbes          bool   `json:"skip_probes,omitempty"`
	ProbeTimeoutSeconds int    `json:"probe_timeout_seconds,omitempty"`
	ProbeConcurrency    int    `json:"probe_concurrency,omitempty"`
	AllowPrivateProbes  bool   `json:"allow_private_probes,omitempty"`
	TargetIP            string `json:"target_ip,omitempty"`
	SourceIP            string `json:"source_ip,omitempty"`
}

// MigrationVerifyReport is the parity report plus whether Re-check is possible.
type MigrationVerifyReport struct {
	migrateverify.Report
	CanRecheck bool `json:"can_recheck"`
}

// VerifyMigration calls POST /api/v1/migrations/verify. It changes nothing but stores the report.
func (c *Client) VerifyMigration(ctx context.Context, req MigrationVerifyRequest) (MigrationVerifyReport, error) {
	var out MigrationVerifyReport
	err := c.do(ctx, http.MethodPost, "/api/v1/migrations/verify", req, &out)
	return out, err
}

// LatestMigrationVerify calls GET /api/v1/migrations/verify/latest.
func (c *Client) LatestMigrationVerify(ctx context.Context) (MigrationVerifyReport, error) {
	var out MigrationVerifyReport
	err := c.do(ctx, http.MethodGet, "/api/v1/migrations/verify/latest", nil, &out)
	return out, err
}

// MigrationPlanReport is the read-only plan of a source platform.
type MigrationPlanReport = migrationplan.Report

// MigrationOverride is a per-item change chosen from a fix.
type MigrationOverride = migrationplan.Override

// MigrationVolumeStat is a volume's file count and size on both hosts.
type MigrationVolumeStat = migrationplan.VolumeStat

// MigrationSourceRequest names the source platform and carries its
// credential. Only selects items by source id or name.
type MigrationSourceRequest struct {
	Platform      string                       `json:"platform"`
	URL           string                       `json:"url,omitempty"`
	Token         string                       `json:"token,omitempty"`
	InsecureTLS   bool                         `json:"insecure_tls,omitempty"`
	AllowPrivate  bool                         `json:"allow_private,omitempty"`
	AllowLoopback bool                         `json:"allow_loopback,omitempty"`
	Only          []string                     `json:"only,omitempty"`
	Collision     string                       `json:"collision,omitempty"`
	Overrides     map[string]MigrationOverride `json:"overrides,omitempty"`
	DryRun        bool                         `json:"dry_run,omitempty"`
}

// MigrationChecklistStep is one step of an app's cutover checklist.
type MigrationChecklistStep struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	At       string   `json:"at,omitempty"`
	Actor    string   `json:"actor,omitempty"`
	Note     string   `json:"note,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// MigrationChecklistVolume carries the commands that measure a volume.
type MigrationChecklistVolume struct {
	Name          string `json:"name"`
	SourceCommand string `json:"source_command,omitempty"`
	TargetCommand string `json:"target_command"`
}

// MigrationChecklist is one app's cutover checklist.
type MigrationChecklist struct {
	App      string                     `json:"app"`
	Platform string                     `json:"platform,omitempty"`
	SourceID string                     `json:"source_id,omitempty"`
	Steps    []MigrationChecklistStep   `json:"steps"`
	Current  string                     `json:"current"`
	Progress int                        `json:"progress"`
	Total    int                        `json:"total"`
	Domains  []string                   `json:"domains,omitempty"`
	Volumes  []MigrationChecklistVolume `json:"volumes,omitempty"`
	Tracked  bool                       `json:"tracked"`
}

// MigrationStepRequest completes or resets one checklist step.
type MigrationStepRequest struct {
	Action    string                `json:"action"`
	Note      string                `json:"note,omitempty"`
	Confirm   bool                  `json:"confirm,omitempty"`
	Databases []string              `json:"databases,omitempty"`
	Volumes   []MigrationVolumeStat `json:"volumes,omitempty"`
}

// PlanMigration calls POST /api/v1/migration/plan. It creates nothing.
func (c *Client) PlanMigration(ctx context.Context, req MigrationSourceRequest) (MigrationPlanReport, error) {
	var out MigrationPlanReport
	err := c.do(ctx, http.MethodPost, "/api/v1/migration/plan", req, &out)
	return out, err
}

// ImportMigration calls POST /api/v1/migration/import for the selected items,
// or previews them when req.DryRun is set.
func (c *Client) ImportMigration(ctx context.Context, req MigrationSourceRequest) (MigrationPlanReport, error) {
	var out MigrationPlanReport
	err := c.do(ctx, http.MethodPost, "/api/v1/migration/import", req, &out)
	return out, err
}

// ListMigrationChecklists calls GET /api/v1/migration/checklists.
func (c *Client) ListMigrationChecklists(ctx context.Context) ([]MigrationChecklist, error) {
	var out struct {
		Checklists []MigrationChecklist `json:"checklists"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/migration/checklists", nil, &out)
	return out.Checklists, err
}

// GetMigrationChecklist calls GET /api/v1/migration/checklists/{app}.
func (c *Client) GetMigrationChecklist(ctx context.Context, app string) (MigrationChecklist, error) {
	var out MigrationChecklist
	err := c.do(ctx, http.MethodGet, "/api/v1/migration/checklists/"+url.PathEscape(app), nil, &out)
	return out, err
}

// StepMigrationChecklist calls POST /api/v1/migration/checklists/{app}/steps/{step}.
func (c *Client) StepMigrationChecklist(ctx context.Context, app, step string, req MigrationStepRequest) (MigrationChecklist, error) {
	var out MigrationChecklist
	err := c.do(ctx, http.MethodPost, "/api/v1/migration/checklists/"+url.PathEscape(app)+"/steps/"+url.PathEscape(step), req, &out)
	return out, err
}
