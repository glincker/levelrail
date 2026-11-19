package apiclient

import (
	"context"
	"net/http"
	"strconv"
)

// StaticSiteLogLine is one build log line tagged with its Pages phase.
type StaticSiteLogLine struct {
	Step   string `json:"step,omitempty"`
	Stream string `json:"stream"`
	Line   string `json:"line"`
	At     string `json:"at,omitempty"`
}

// StaticSiteDeployLog is the response of GET /api/v1/static-sites/{name}/deploys/{id}/log.
type StaticSiteDeployLog struct {
	DeployID string              `json:"deploy_id"`
	Status   string              `json:"status"`
	Running  bool                `json:"running"`
	Lines    []StaticSiteLogLine `json:"lines"`
}

// GetStaticSiteDeployLog fetches the stored build log; tail > 0 keeps only the last tail lines.
func (c *Client) GetStaticSiteDeployLog(ctx context.Context, name, deployID string, tail int) (*StaticSiteDeployLog, error) {
	path := "/api/v1/static-sites/" + PathEscape(name) + "/deploys/" + PathEscape(deployID) + "/log"
	if tail > 0 {
		path += "?tail=" + strconv.Itoa(tail)
	}
	var out StaticSiteDeployLog
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StreamStaticSiteDeployLog follows a deploy's log over SSE until ctx ends or the server closes.
func (c *Client) StreamStaticSiteDeployLog(ctx context.Context, name, deployID string, onEntry func(LogStreamEntry) error) error {
	path := "/api/v1/static-sites/" + PathEscape(name) + "/deploys/" + PathEscape(deployID) + "/log/stream"
	return c.streamLogEvents(ctx, path, onEntry)
}
