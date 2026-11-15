package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// DeployStage is one build stage of a deploy attempt.
type DeployStage struct {
	Stage          string     `json:"stage"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	DurationMS     int64      `json:"duration_ms"`
	Cache          string     `json:"cache,omitempty"`
	Vertices       int        `json:"vertices"`
	CachedVertices int        `json:"cached_vertices"`
	LogBytes       int64      `json:"log_bytes"`
}

// DeployStages is the build stage view of one deploy attempt.
type DeployStages struct {
	DeployID string        `json:"deploy_id"`
	Status   string        `json:"status"`
	Stages   []DeployStage `json:"stages"`
	WallMS   int64         `json:"wall_ms"`
	StagesMS int64         `json:"stages_ms"`
}

// GetDeployStages calls GET /api/v1/apps/{name}/deploys/{deployID}/stages.
func (c *Client) GetDeployStages(ctx context.Context, name, deployID string) (DeployStages, error) {
	var out DeployStages
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/deploys/"+PathEscape(deployID)+"/stages", nil, &out)
	return out, err
}

// StreamDeployStageLog tails one build stage's log lines of a deploy attempt.
func (c *Client) StreamDeployStageLog(ctx context.Context, name, deployID, stage string, onEntry func(LogStreamEntry) error) error {
	path := "/api/v1/apps/" + PathEscape(name) + "/deploys/" + PathEscape(deployID) + "/logs?" + url.Values{"stage": {stage}}.Encode()
	return c.streamLogEvents(ctx, path, onEntry)
}
