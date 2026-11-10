package apiclient

import (
	"context"
	"net/http"
)

// NodeThresholdValues are alert percents; a nil field means unset (override) or unknown.
type NodeThresholdValues struct {
	CPUPercent    *int `json:"cpu_percent"`
	MemoryPercent *int `json:"memory_percent"`
	DiskPercent   *int `json:"disk_percent"`
}

// NodeThresholds is the GET/PUT /api/v1/nodes/{id}/thresholds response.
type NodeThresholds struct {
	NodeID             string              `json:"node_id"`
	Defaults           NodeThresholdValues `json:"defaults"`
	Override           NodeThresholdValues `json:"override"`
	Effective          NodeThresholdValues `json:"effective"`
	SustainedIntervals int                 `json:"sustained_intervals"`
}

// GetNodeThresholds calls GET /api/v1/nodes/{id}/thresholds.
func (c *Client) GetNodeThresholds(ctx context.Context, id string) (NodeThresholds, error) {
	var out NodeThresholds
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/thresholds", nil, &out)
	return out, err
}

// SetNodeThresholds calls PUT /api/v1/nodes/{id}/thresholds; the body replaces the whole override.
func (c *Client) SetNodeThresholds(ctx context.Context, id string, override NodeThresholdValues) (NodeThresholds, error) {
	var out NodeThresholds
	err := c.do(ctx, http.MethodPut, nodePath(id)+"/thresholds", override, &out)
	return out, err
}
