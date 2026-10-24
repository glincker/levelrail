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
