package apiclient

import (
	"context"
	"time"
)

// AppHealthExplainCheck mirrors internal/api's healthExplainCheck.
type AppHealthExplainCheck struct {
	Target     string    `json:"target"`
	Error      string    `json:"error,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	Since      time.Time `json:"since"`
	LastProbed time.Time `json:"last_probed"`
}

// AppHealthExplainDeploy mirrors internal/api's healthExplainDeploy.
type AppHealthExplainDeploy struct {
	ID        string    `json:"id"`
	Image     string    `json:"image"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error,omitempty"`
}

// AppHealthExplainNextStep mirrors internal/api's healthExplainNextStep.
type AppHealthExplainNextStep struct {
	Kind    string `json:"kind"`
	Preset  string `json:"preset,omitempty"`
	Command string `json:"command,omitempty"`
}

// AppHealthExplainResource mirrors internal/api's appHealthExplainResource.
type AppHealthExplainResource struct {
	AppName      string                   `json:"app_name"`
	State        string                   `json:"state"`
	FailingCheck *AppHealthExplainCheck   `json:"failing_check,omitempty"`
	Restarts     *int                     `json:"restarts,omitempty"`
	Crashloop    bool                     `json:"crashloop"`
	LastDeploy   *AppHealthExplainDeploy  `json:"last_deploy,omitempty"`
	LogError     string                   `json:"log_error,omitempty"`
	NextStep     AppHealthExplainNextStep `json:"next_step"`
	ComputedAt   time.Time                `json:"computed_at"`
}

// GetAppHealthExplain calls GET /api/v1/apps/{name}/health-explain.
func (c *Client) GetAppHealthExplain(ctx context.Context, name string) (AppHealthExplainResource, error) {
	return getJSON[AppHealthExplainResource](ctx, c, "/api/v1/apps/"+PathEscape(name)+"/health-explain")
}
