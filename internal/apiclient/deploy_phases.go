package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DeployPhase is one stage of a deploy attempt.
type DeployPhase struct {
	Phase      string     `json:"phase"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMS *int64     `json:"duration_ms"`
	TypicalMS  *int64     `json:"typical_ms"`
}

// DeployPhases is the GET /api/v1/deployments/{id}/phases response.
type DeployPhases struct {
	DeployID       string        `json:"deploy_id"`
	App            string        `json:"app"`
	Status         string        `json:"status"`
	ElapsedMS      int64         `json:"elapsed_ms"`
	Phases         []DeployPhase `json:"phases"`
	ETAMS          *int64        `json:"eta_ms"`
	TypicalTotalMS *int64        `json:"typical_total_ms"`
	Samples        int           `json:"samples"`
}

// GetDeployPhases calls GET /api/v1/deployments/{id}/phases.
func (c *Client) GetDeployPhases(ctx context.Context, deployID string) (DeployPhases, error) {
	var out DeployPhases
	err := c.do(ctx, http.MethodGet, "/api/v1/deployments/"+PathEscape(deployID)+"/phases", nil, &out)
	return out, err
}
