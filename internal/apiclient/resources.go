package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ConsoleResource is one row of GET /api/v1/resources.
type ConsoleResource struct {
	ID     string   `json:"id,omitempty"`
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Detail string   `json:"detail,omitempty"`
	Status string   `json:"status,omitempty"`
	Tags   []string `json:"tags"`
}

// ConsoleList is the explorer response.
type ConsoleList struct {
	Items  []ConsoleResource `json:"items"`
	Total  int               `json:"total"`
	Counts map[string]int    `json:"counts"`
}

// ConsoleQuery filters the explorer list.
type ConsoleQuery struct {
	Text  string
	Kind  string
	Tags  []string
	Limit int
}

// ConsoleRef names one resource in a bulk request.
type ConsoleRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// ConsoleBulkTagRequest is the body of POST /api/v1/resources/tags/bulk.
type ConsoleBulkTagRequest struct {
	Action    string       `json:"action"`
	Tag       string       `json:"tag"`
	Resources []ConsoleRef `json:"resources"`
	DryRun    bool         `json:"dry_run,omitempty"`
}

// ConsoleBulkResult is one resource outcome of a bulk tag call.
type ConsoleBulkResult struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ConsoleBulkTagResponse is the bulk tag outcome.
type ConsoleBulkTagResponse struct {
	Action  string              `json:"action"`
	Tag     string              `json:"tag"`
	DryRun  bool                `json:"dry_run"`
	Results []ConsoleBulkResult `json:"results"`
	Counts  map[string]int      `json:"counts"`
}

// ConsoleView is a saved explorer view.
type ConsoleView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	CreatedAt time.Time `json:"created_at"`
}

// ListConsoleResources calls GET /api/v1/resources.
func (c *Client) ListConsoleResources(ctx context.Context, q ConsoleQuery) (ConsoleList, error) {
	v := url.Values{}
	if q.Text != "" {
		v.Set("q", q.Text)
	}
	if q.Kind != "" {
		v.Set("kind", q.Kind)
	}
	for _, t := range q.Tags {
		v.Add("tag", t)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/api/v1/resources"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out ConsoleList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// BulkTagResources calls POST /api/v1/resources/tags/bulk.
func (c *Client) BulkTagResources(ctx context.Context, req ConsoleBulkTagRequest) (ConsoleBulkTagResponse, error) {
	var out ConsoleBulkTagResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/resources/tags/bulk", req, &out)
	return out, err
}

// ListConsoleViews calls GET /api/v1/resources/views.
func (c *Client) ListConsoleViews(ctx context.Context) ([]ConsoleView, error) {
	var out []ConsoleView
	err := c.do(ctx, http.MethodGet, "/api/v1/resources/views", nil, &out)
	return out, err
}

// SaveConsoleView calls POST /api/v1/resources/views.
func (c *Client) SaveConsoleView(ctx context.Context, name, query string) (ConsoleView, error) {
	var out ConsoleView
	err := c.do(ctx, http.MethodPost, "/api/v1/resources/views", ConsoleView{Name: name, Query: query}, &out)
	return out, err
}

// DeleteConsoleView calls DELETE /api/v1/resources/views/{id}.
func (c *Client) DeleteConsoleView(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/resources/views/"+PathEscape(id), nil, nil)
}
