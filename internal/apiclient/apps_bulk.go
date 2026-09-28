package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// BulkAppsRequest is POST /api/v1/apps/bulk's body.
type BulkAppsRequest struct {
	Action       string   `json:"action"`
	Names        []string `json:"names,omitempty"`
	Tag          string   `json:"tag,omitempty"`
	Environment  string   `json:"environment,omitempty"`
	Value        string   `json:"value,omitempty"`
	DryRun       bool     `json:"dry_run,omitempty"`
	ConfirmNames []string `json:"confirm_names,omitempty"`
}

// BulkAppResult is one app's outcome in a bulk response.
type BulkAppResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// BulkAppsResponse is the 207 body of POST /api/v1/apps/bulk.
type BulkAppsResponse struct {
	Action  string          `json:"action"`
	DryRun  bool            `json:"dry_run"`
	Results []BulkAppResult `json:"results"`
	Counts  map[string]int  `json:"counts"`
}

// BulkApps calls POST /api/v1/apps/bulk.
func (c *Client) BulkApps(ctx context.Context, req BulkAppsRequest) (BulkAppsResponse, error) {
	var out BulkAppsResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/bulk", req, &out)
	return out, err
}

// AppsSummary is GET /api/v1/apps-summary's body.
type AppsSummary struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Failing   int `json:"failing"`
	Deploying int `json:"deploying"`
	Stopped   int `json:"stopped"`
	Unknown   int `json:"unknown"`
}

// AppListQuery narrows GET /api/v1/apps and /api/v1/apps-summary.
type AppListQuery struct {
	Q, Project, Environment string
	Tags                    []string
	Limit, Offset           int
}

func (q AppListQuery) values() url.Values {
	v := url.Values{}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Project != "" {
		v.Set("project", q.Project)
	}
	if q.Environment != "" {
		v.Set("environment", q.Environment)
	}
	for _, t := range q.Tags {
		v.Add("tag", t)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	return v
}

// ListAppsFiltered calls GET /api/v1/apps with filters.
func (c *Client) ListAppsFiltered(ctx context.Context, q AppListQuery) ([]AppResource, error) {
	var out []AppResource
	path := "/api/v1/apps"
	if enc := q.values().Encode(); enc != "" {
		path += "?" + enc
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetAppsSummary calls GET /api/v1/apps-summary.
func (c *Client) GetAppsSummary(ctx context.Context, q AppListQuery) (AppsSummary, error) {
	var out AppsSummary
	path := "/api/v1/apps-summary"
	if enc := q.values().Encode(); enc != "" {
		path += "?" + enc
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
