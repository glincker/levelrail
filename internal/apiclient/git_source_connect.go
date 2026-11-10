package apiclient

import (
	"context"
	"net/http"
)

// GitSourcePlanPreserved is what attaching a repository keeps on the app.
type GitSourcePlanPreserved struct {
	EnvVars      int      `json:"env_vars"`
	SecretEnv    int      `json:"secret_env"`
	Domains      []string `json:"domains"`
	Volumes      []string `json:"volumes"`
	Replicas     int      `json:"replicas"`
	RunningImage string   `json:"running_image,omitempty"`
}

// GitSourcePlanSuggestion is the build the plan suggests for the repository.
type GitSourcePlanSuggestion struct {
	BuildType     string `json:"build_type"`
	BuildPath     string `json:"build_path,omitempty"`
	BaseDirectory string `json:"base_directory,omitempty"`
	BuildCommand  string `json:"build_command,omitempty"`
	PublishDir    string `json:"publish_dir,omitempty"`
	Framework     string `json:"framework,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// GitSourcePlan is POST /api/v1/apps/{name}/git-source/plan's response.
type GitSourcePlan struct {
	App           string                   `json:"app"`
	CurrentSource string                   `json:"current_source"`
	CurrentRepo   string                   `json:"current_repo,omitempty"`
	Static        bool                     `json:"static"`
	Branch        string                   `json:"branch,omitempty"`
	Preserved     GitSourcePlanPreserved   `json:"preserved"`
	Suggestion    *GitSourcePlanSuggestion `json:"suggestion,omitempty"`
	DetectError   string                   `json:"detect_error,omitempty"`
	Warnings      []string                 `json:"warnings"`
	Group         []GitSourcePlanMember    `json:"group"`
}

// GitSourcePlanMember is another service of the same app and what the switch does to it.
type GitSourcePlanMember struct {
	Name          string `json:"name"`
	CurrentSource string `json:"current_source"`
	Effect        string `json:"effect"`
}

// GitSourcePlanOptions narrows a plan: the repo, its branch and the provider connection that reads it.
type GitSourcePlanOptions struct {
	RepoURL      string
	Branch       string
	ConnectionID string
}

// GitSourceDeployResult is POST /api/v1/apps/{name}/git-source/deploy's response.
type GitSourceDeployResult struct {
	App     string `json:"app"`
	Branch  string `json:"branch"`
	Started bool   `json:"started"`
}

// PlanGitSource calls POST /api/v1/apps/{name}/git-source/plan.
func (c *Client) PlanGitSource(ctx context.Context, name string, opts GitSourcePlanOptions) (GitSourcePlan, error) {
	var out GitSourcePlan
	body := map[string]string{"repo_url": opts.RepoURL, "branch": opts.Branch}
	if opts.ConnectionID != "" {
		body["connection_id"] = opts.ConnectionID
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/git-source/plan", body, &out)
	return out, err
}

// DeployGitSource calls POST /api/v1/apps/{name}/git-source/deploy.
func (c *Client) DeployGitSource(ctx context.Context, name, branch string) (GitSourceDeployResult, error) {
	var out GitSourceDeployResult
	body := map[string]string{}
	if branch != "" {
		body["branch"] = branch
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/git-source/deploy", body, &out)
	return out, err
}
