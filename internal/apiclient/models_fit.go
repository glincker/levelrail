package apiclient

import (
	"context"
	"net/http"
)

// ModelFitRequest is the body of POST /api/v1/models/fit. NodeID nil rates
// every GPU node.
type ModelFitRequest struct {
	Engine        string   `json:"engine"`
	Model         string   `json:"model"`
	Quantization  string   `json:"quantization,omitempty"`
	ContextLength int      `json:"context_length,omitempty"`
	GPUCount      int      `json:"gpu_count,omitempty"`
	GPUDeviceIDs  []string `json:"gpu_device_ids,omitempty"`
	WeightsBytes  int64    `json:"weights_bytes,omitempty"`
	NodeID        *string  `json:"node_id,omitempty"`
}

// NodeFit mirrors internal/models.NodeFit.
type NodeFit struct {
	NodeID         string   `json:"node_id"`
	Name           string   `json:"name"`
	IsLocal        bool     `json:"is_local"`
	Eligible       bool     `json:"eligible"`
	Current        bool     `json:"current"`
	GPUs           int      `json:"gpus"`
	TotalBytes     int64    `json:"total_bytes"`
	ReservedBytes  int64    `json:"reserved_bytes"`
	Verdict        string   `json:"verdict"`
	WeightsSource  string   `json:"weights_source"`
	WeightsBytes   int64    `json:"weights_bytes"`
	KVBytes        int64    `json:"kv_bytes"`
	OverheadBytes  int64    `json:"overhead_bytes"`
	NeedBytes      int64    `json:"need_bytes"`
	FreeBytes      int64    `json:"free_bytes"`
	ContextTokens  int      `json:"context_tokens"`
	ContextAssumed bool     `json:"context_assumed"`
	Arithmetic     string   `json:"arithmetic"`
	Reason         string   `json:"reason,omitempty"`
	Suggestions    []string `json:"suggestions"`
}

// ModelFitReport mirrors internal/models.FitReport.
type ModelFitReport struct {
	Model string    `json:"model,omitempty"`
	Nodes []NodeFit `json:"nodes"`
	Note  string    `json:"note"`
}

// CheckModelFit calls POST /api/v1/models/fit.
func (c *Client) CheckModelFit(ctx context.Context, req ModelFitRequest) (ModelFitReport, error) {
	var out ModelFitReport
	err := c.do(ctx, http.MethodPost, "/api/v1/models/fit", req, &out)
	return out, err
}

// GetModelFit calls GET /api/v1/models/{name}/fit.
func (c *Client) GetModelFit(ctx context.Context, name string) (ModelFitReport, error) {
	var out ModelFitReport
	err := c.do(ctx, http.MethodGet, "/api/v1/models/"+PathEscape(name)+"/fit", nil, &out)
	return out, err
}
