package apiclient

import (
	"context"
	"net/http"
)

// ImportRepoPlanRequest is the body of POST /api/v1/imports/repo/plan.
type ImportRepoPlanRequest struct {
	RepoURL       string `json:"repo_url"`
	Branch        string `json:"branch,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
}

// RepoPlanWarning mirrors internal/repoplan's Warning.
type RepoPlanWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RepoPlanRow mirrors internal/repoplan's Row.
type RepoPlanRow struct {
	Name        string            `json:"name"`
	PackageName string            `json:"package_name"`
	Directory   string            `json:"directory"`
	Port        *int              `json:"port"`
	PortSource  string            `json:"port_source,omitempty"`
	BuildKind   string            `json:"build_kind"`
	Dockerfile  string            `json:"dockerfile,omitempty"`
	WatchPaths  []string          `json:"watch_paths"`
	DependsOn   []string          `json:"depends_on"`
	Warnings    []RepoPlanWarning `json:"warnings"`
}

// RepoPlan mirrors internal/repoplan's Plan.
type RepoPlan struct {
	RepoURL       string            `json:"repo_url"`
	Branch        string            `json:"branch"`
	BaseDirectory string            `json:"base_directory,omitempty"`
	Tool          string            `json:"tool,omitempty"`
	PackageCap    int               `json:"package_cap"`
	PackagesFound int               `json:"packages_found"`
	Packages      []RepoPlanRow     `json:"packages"`
	Warnings      []RepoPlanWarning `json:"warnings"`
	Totals        struct {
		Packages int `json:"packages"`
		Warnings int `json:"warnings"`
		Flagged  int `json:"flagged"`
	} `json:"totals"`
}

// PlanImportRepo calls POST /api/v1/imports/repo/plan. It creates nothing.
func (c *Client) PlanImportRepo(ctx context.Context, req ImportRepoPlanRequest) (RepoPlan, error) {
	var out RepoPlan
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/repo/plan", req, &out)
	return out, err
}

// ImportRepoApplyPackage is one reviewed package of an apply request.
type ImportRepoApplyPackage struct {
	Package       string            `json:"package"`
	Name          string            `json:"name"`
	Directory     string            `json:"directory"`
	Port          int               `json:"port,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	SecretEnvKeys []string          `json:"secret_env_keys,omitempty"`
	ProjectID     string            `json:"project_id,omitempty"`
	EnvironmentID string            `json:"environment_id,omitempty"`
	Deploy        bool              `json:"deploy,omitempty"`
}

// ImportRepoApplyRequest is the body of POST /api/v1/imports/repo/apply.
type ImportRepoApplyRequest struct {
	RepoURL       string                   `json:"repo_url"`
	Branch        string                   `json:"branch,omitempty"`
	BaseDirectory string                   `json:"base_directory,omitempty"`
	Packages      []ImportRepoApplyPackage `json:"packages"`
	DryRun        bool                     `json:"dry_run,omitempty"`
	Confirm       bool                     `json:"confirm,omitempty"`
}

// ImportRepoApplyResult is the outcome for one package.
type ImportRepoApplyResult struct {
	Package     string `json:"package"`
	Name        string `json:"name"`
	Directory   string `json:"directory"`
	Status      string `json:"status"`
	Code        string `json:"code,omitempty"`
	Message     string `json:"message,omitempty"`
	AppID       string `json:"app_id,omitempty"`
	DeployID    string `json:"deploy_id,omitempty"`
	DeployError string `json:"deploy_error,omitempty"`
}

// ImportRepoApplyResponse is the per-app report of an apply or dry run.
type ImportRepoApplyResponse struct {
	RepoURL           string                  `json:"repo_url"`
	Branch            string                  `json:"branch"`
	DryRun            bool                    `json:"dry_run"`
	NeedsConfirmation bool                    `json:"needs_confirmation"`
	ConfirmAbove      int                     `json:"confirm_above"`
	Counts            map[string]int          `json:"counts"`
	Results           []ImportRepoApplyResult `json:"results"`
}

// ApplyImportRepo calls POST /api/v1/imports/repo/apply.
func (c *Client) ApplyImportRepo(ctx context.Context, req ImportRepoApplyRequest) (ImportRepoApplyResponse, error) {
	var out ImportRepoApplyResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/repo/apply", req, &out)
	return out, err
}
