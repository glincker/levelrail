package apiclient

import (
	"context"
	"net/http"
)

// LogPipelineRule mirrors telemetry.PipelineRule.
type LogPipelineRule struct {
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
	Replace string `json:"replace,omitempty"`
}

// LogPipeline is an app's ordered rule list.
type LogPipeline struct {
	Rules []LogPipelineRule `json:"rules"`
}

// LogPipelinePreviewLine is the outcome of one sample line.
type LogPipelinePreviewLine struct {
	Message string            `json:"message"`
	Dropped bool              `json:"dropped"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// LogPipelinePreview is the response of PreviewLogPipeline.
type LogPipelinePreview struct {
	Lines []LogPipelinePreviewLine `json:"lines"`
}

func logPipelinePath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/log-pipeline"
}

// GetLogPipeline calls GET /api/v1/apps/{name}/log-pipeline.
func (c *Client) GetLogPipeline(ctx context.Context, app string) (LogPipeline, error) {
	var out LogPipeline
	err := c.do(ctx, http.MethodGet, logPipelinePath(app), nil, &out)
	return out, err
}

// SetLogPipeline calls PUT /api/v1/apps/{name}/log-pipeline.
func (c *Client) SetLogPipeline(ctx context.Context, app string, rules []LogPipelineRule) (LogPipeline, error) {
	var out LogPipeline
	err := c.do(ctx, http.MethodPut, logPipelinePath(app), LogPipeline{Rules: rules}, &out)
	return out, err
}

// DeleteLogPipeline calls DELETE /api/v1/apps/{name}/log-pipeline.
func (c *Client) DeleteLogPipeline(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, logPipelinePath(app), nil, nil)
}

// PreviewLogPipeline calls POST /api/v1/apps/{name}/log-pipeline/preview.
func (c *Client) PreviewLogPipeline(ctx context.Context, app string, rules []LogPipelineRule, lines []string) (LogPipelinePreview, error) {
	body := struct {
		Rules []LogPipelineRule `json:"rules"`
		Lines []string          `json:"lines"`
	}{rules, lines}
	var out LogPipelinePreview
	err := c.do(ctx, http.MethodPost, logPipelinePath(app)+"/preview", body, &out)
	return out, err
}
