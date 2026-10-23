package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// FunctionInvokeRequest is one test call sent through POST /api/v1/functions/{name}/invoke.
type FunctionInvokeRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// FunctionInvokeResult is the function's answer to a test call or a replay.
type FunctionInvokeResult struct {
	RequestID     string              `json:"request_id"`
	Status        int                 `json:"status"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	BodyTruncated bool                `json:"body_truncated"`
	DurationMs    float64             `json:"duration_ms"`
	ColdStart     bool                `json:"cold_start"`
	InputStored   bool                `json:"input_stored"`
}

// InvokeFunction runs a test call through the function's real route, wake included.
func (c *Client) InvokeFunction(ctx context.Context, name string, req FunctionInvokeRequest) (FunctionInvokeResult, error) {
	var out FunctionInvokeResult
	err := c.do(ctx, http.MethodPost, "/api/v1/functions/"+url.PathEscape(name)+"/invoke", req, &out)
	return out, err
}

// ReplayFunctionInvocation repeats a recorded call whose input was stored.
func (c *Client) ReplayFunctionInvocation(ctx context.Context, name, id string) (FunctionInvokeResult, error) {
	var out FunctionInvokeResult
	err := c.do(ctx, http.MethodPost, "/api/v1/functions/"+url.PathEscape(name)+"/invocations/"+url.PathEscape(id)+"/replay", nil, &out)
	return out, err
}

// FunctionInvocation is one recorded call of a function.
type FunctionInvocation struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	DurationMs float64   `json:"duration_ms"`
	ColdStart  bool      `json:"cold_start"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	Replayable bool      `json:"replayable"`
}

// FunctionInvocations is the GET /api/v1/functions/{name}/invocations response.
type FunctionInvocations struct {
	Invocations []FunctionInvocation `json:"invocations"`
	Capture     bool                 `json:"capture"`
}

// ListFunctionInvocations returns the function's recent calls, newest first; limit 0 uses the server default.
func (c *Client) ListFunctionInvocations(ctx context.Context, name string, limit int) (FunctionInvocations, error) {
	path := "/api/v1/functions/" + url.PathEscape(name) + "/invocations"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out FunctionInvocations
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
