package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// ProjectArchiveExportRequest is the body of POST /api/v1/projects/{id}/export.
type ProjectArchiveExportRequest struct {
	IncludeSecrets bool   `json:"include_secrets"`
	Passphrase     string `json:"passphrase,omitempty"`
	IncludeData    bool   `json:"include_data"`
}

// ProjectImportRequest is the body of POST /api/v1/projects/import and /plan.
// Archive is the .lrx file, base64 on the wire.
type ProjectImportRequest struct {
	Archive    []byte `json:"archive"`
	Passphrase string `json:"passphrase,omitempty"`
	Rename     string `json:"rename,omitempty"`
	// ExpectedPlanHash is the hash from the plan call, required to apply.
	ExpectedPlanHash string `json:"expected_plan_hash,omitempty"`
}

// ProjectImportItem is one resource an import creates.
type ProjectImportItem struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ProjectImportConflict is one thing that blocks an import.
type ProjectImportConflict struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// ProjectImportPlan is the dry run of an import.
type ProjectImportPlan struct {
	Project         string                  `json:"project"`
	SourceProject   string                  `json:"source_project"`
	SourceVersion   string                  `json:"source_version"`
	SecretsIncluded bool                    `json:"secrets_included"`
	SecretsApplied  bool                    `json:"secrets_applied"`
	DataFiles       []string                `json:"data_files,omitempty"`
	Creates         []ProjectImportItem     `json:"creates"`
	Conflicts       []ProjectImportConflict `json:"conflicts"`
	Manual          []string                `json:"manual_steps"`
	Warnings        []string                `json:"warnings,omitempty"`
	Hash            string                  `json:"hash,omitempty"`
}

// ProjectImportResponse is the plan, plus the apply result for a real import.
type ProjectImportResponse struct {
	Plan   ProjectImportPlan `json:"plan"`
	Result *iac.ApplyResult  `json:"result,omitempty"`
}

// ExportProjectArchiveTo streams the .lrx archive of a project into w, so a
// dump with data never has to fit in memory.
func (c *Client) ExportProjectArchiveTo(ctx context.Context, project string, req ProjectArchiveExportRequest, w io.Writer) (int64, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	path := "/api/v1/projects/" + PathEscape(project) + "/export"
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload)) //nolint:gosec // c.baseURL is the operator-supplied API target
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	hr.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		hr.Header.Set(headerAuthorization, bearerPrefix+c.token)
	}
	if c.userAgent != "" {
		hr.Header.Set("User-Agent", c.userAgent)
	}
	start := time.Now()
	resp, err := c.hc.Do(hr) //nolint:gosec // same target as above
	if err != nil {
		TraceRequest(http.MethodPost, c.baseURL+path, "", time.Since(start), err)
		return 0, fmt.Errorf("request POST %s: %w", c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	TraceRequest(http.MethodPost, c.baseURL+path, resp.Status, time.Since(start), nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return 0, &APIError{StatusCode: resp.StatusCode, Message: ExtractErrorMessage(data), RetryAfter: retryAfterHeader(resp.Header)}
	}
	return io.Copy(w, resp.Body)
}

// PlanProjectImport calls POST /api/v1/projects/import/plan.
func (c *Client) PlanProjectImport(ctx context.Context, req ProjectImportRequest) (*ProjectImportResponse, error) {
	var out ProjectImportResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/projects/import/plan", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ImportProject calls POST /api/v1/projects/import.
func (c *Client) ImportProject(ctx context.Context, req ProjectImportRequest) (*ProjectImportResponse, error) {
	var out ProjectImportResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/projects/import", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
