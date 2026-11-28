package apiclient

import "context"

// DeployWhyTemplate is one log cluster the deploy has and its last good deploy lacks.
type DeployWhyTemplate struct {
	TemplateID string `json:"template_id"`
	Template   string `json:"template"`
	Count      int    `json:"count"`
	Sample     string `json:"sample"`
}

// DeployWhyHealth is the last probe result of a deploy.
type DeployWhyHealth struct {
	Target     string `json:"target"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// DeployWhy is the verdict from GET /api/v1/deploys/{id}/why.
type DeployWhy struct {
	DeployID         string              `json:"deploy_id"`
	Verdict          string              `json:"verdict"`
	ExitCode         *int                `json:"exit_code,omitempty"`
	OOMKilled        bool                `json:"oom_killed,omitempty"`
	Health           *DeployWhyHealth    `json:"health,omitempty"`
	BaselineID       string              `json:"baseline_deploy_id,omitempty"`
	NewTemplateCount int                 `json:"new_template_count"`
	NewTemplates     []DeployWhyTemplate `json:"new_templates"`
	Summary          []string            `json:"summary,omitempty"`
}

// DeployWhy calls GET /api/v1/deploys/{id}/why.
func (c *Client) DeployWhy(ctx context.Context, id string) (DeployWhy, error) {
	return getJSON[DeployWhy](ctx, c, "/api/v1/deploys/"+PathEscape(id)+"/why")
}
