package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// TimelineDeploy is one deploy bar of GET /api/v1/deployments/timeline.
type TimelineDeploy struct {
	ID            string     `json:"id"`
	App           string     `json:"app"`
	Status        string     `json:"status"`
	Trigger       string     `json:"trigger"`
	Environment   string     `json:"environment"`
	CommitSHA     string     `json:"commit_sha"`
	CommitMessage string     `json:"commit_message"`
	Author        string     `json:"author"`
	Strategy      string     `json:"strategy"`
	ProjectID     string     `json:"project_id"`
	NodeID        string     `json:"node_id"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	DurationMS    *int64     `json:"duration_ms"`
	RollbackOf    *string    `json:"rollback_of"`
	IsLive        bool       `json:"is_live"`
}

// TimelinePage is one page of the fleet deploy timeline.
type TimelinePage struct {
	Since      time.Time        `json:"since"`
	Until      time.Time        `json:"until"`
	Items      []TimelineDeploy `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

// DeployTimeline calls GET /api/v1/deployments/timeline with the given
// query (since, until, app, project, node, status, limit, cursor).
func (c *Client) DeployTimeline(ctx context.Context, q url.Values) (TimelinePage, error) {
	path := "/api/v1/deployments/timeline"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out TimelinePage
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
