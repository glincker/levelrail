package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// AnalysisReportSummary is one row of an app's analysis report list.
type AnalysisReportSummary struct {
	ID          string    `json:"id"`
	App         string    `json:"app"`
	TriggerKind string    `json:"trigger_kind"`
	TriggerID   string    `json:"trigger_id"`
	Status      string    `json:"status"`
	Summary     string    `json:"summary"`
	CreatedAt   time.Time `json:"created_at"`
}

// AnalysisReport is a full stored report; Report is the structured body.
type AnalysisReport struct {
	AnalysisReportSummary
	SchemaVersion int             `json:"schema_version"`
	Report        json.RawMessage `json:"report"`
}

// ListAnalysisReports calls GET /api/v1/apps/{name}/analysis/reports.
func (c *Client) ListAnalysisReports(ctx context.Context, app string) ([]AnalysisReportSummary, error) {
	var out []AnalysisReportSummary
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/analysis/reports", nil, &out)
	return out, err
}

// GetAnalysisReport calls GET /api/v1/analysis/reports/{id}.
func (c *Client) GetAnalysisReport(ctx context.Context, id string) (AnalysisReport, error) {
	var out AnalysisReport
	err := c.do(ctx, http.MethodGet, "/api/v1/analysis/reports/"+PathEscape(id), nil, &out)
	return out, err
}

// RunAnalysis calls POST /api/v1/apps/{name}/analysis/reports and returns the new report.
func (c *Client) RunAnalysis(ctx context.Context, app string) (AnalysisReport, error) {
	var out AnalysisReport
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(app)+"/analysis/reports", nil, &out)
	return out, err
}
