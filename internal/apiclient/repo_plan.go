package apiclient

import (
	"context"
	"net/http"
)

// RepoPlanViewRequest is the body of POST /api/v1/apps/repo-plan.
type RepoPlanViewRequest struct {
	RepoURL       string `json:"repo_url"`
	Ref           string `json:"ref,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
}

// RepoPlanViewEnv mirrors internal/appwizard's PlanEnv. It carries no value.
type RepoPlanViewEnv struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Required    bool   `json:"required"`
	LooksSecret bool   `json:"looks_secret"`
	HasDefault  bool   `json:"has_default_hint"`
}

// RepoPlanView mirrors internal/appwizard's PlanView.
type RepoPlanView struct {
	RepoURL      string `json:"repo_url"`
	Ref          string `json:"ref"`
	AppDirectory string `json:"app_directory,omitempty"`
	Monorepo     bool   `json:"monorepo"`
	Stack        struct {
		Runtime    string `json:"runtime"`
		Framework  string `json:"framework,omitempty"`
		Language   string `json:"language,omitempty"`
		Confidence string `json:"confidence"`
		Evidence   []struct {
			Signal string `json:"signal"`
			Detail string `json:"detail"`
		} `json:"evidence"`
	} `json:"stack"`
	Build struct {
		Type           string `json:"type"`
		DockerfilePath string `json:"dockerfile_path,omitempty"`
		BaseDirectory  string `json:"base_directory,omitempty"`
		BuildCommand   string `json:"build_command,omitempty"`
		StartCommand   string `json:"start_command,omitempty"`
		OutputDir      string `json:"output_dir,omitempty"`
		Evidence       string `json:"evidence"`
	} `json:"build"`
	Port struct {
		Value    int    `json:"value"`
		Evidence string `json:"evidence"`
	} `json:"port"`
	Health struct {
		Path     string `json:"path"`
		Evidence string `json:"evidence"`
	} `json:"health"`
	Env  []RepoPlanViewEnv `json:"env"`
	Spec struct {
		Name string `json:"name"`
		YAML string `json:"yaml"`
	} `json:"spec"`
	Warnings []RepoPlanWarning `json:"warnings"`
	Next     struct {
		Method string `json:"method"`
		Path   string `json:"path"`
	} `json:"next"`
}

// PlanRepoView calls POST /api/v1/apps/repo-plan. It creates nothing.
func (c *Client) PlanRepoView(ctx context.Context, req RepoPlanViewRequest) (RepoPlanView, error) {
	var out RepoPlanView
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/repo-plan", req, &out)
	return out, err
}
