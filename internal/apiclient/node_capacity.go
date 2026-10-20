package apiclient

import (
	"context"
	"net/http"
	"time"
)

// NodeCapacityMetric is one metric of GET /api/v1/nodes/{id}/capacity.
type NodeCapacityMetric struct {
	Name    string  `json:"name"`
	Level   string  `json:"level"`
	Percent float64 `json:"percent"`
	Free    uint64  `json:"free"`
	Total   uint64  `json:"total"`
}

// NodeCapacityStatus is a node's verdict across disk, inodes, memory and swap.
type NodeCapacityStatus struct {
	Metrics []NodeCapacityMetric `json:"metrics"`
	Worst   string               `json:"worst"`
	Reason  string               `json:"reason,omitempty"`
}

// NodeGuardrailSettings are a node's threshold overrides; nil uses the default.
type NodeGuardrailSettings struct {
	DiskWarnFreePct  *float64 `json:"disk_warn_free_pct"`
	DiskCritFreePct  *float64 `json:"disk_crit_free_pct"`
	InodeWarnFreePct *float64 `json:"inode_warn_free_pct"`
	InodeCritFreePct *float64 `json:"inode_crit_free_pct"`
	MemWarnAvailPct  *float64 `json:"mem_warn_avail_pct"`
	MemCritAvailPct  *float64 `json:"mem_crit_avail_pct"`
	SwapWarnUsedPct  *float64 `json:"swap_warn_used_pct"`
	SwapCritUsedPct  *float64 `json:"swap_crit_used_pct"`
	AutoClean        bool     `json:"auto_clean"`
}

// NodeCapacityResource is GET /api/v1/nodes/{id}/capacity.
type NodeCapacityResource struct {
	NodeID           string                `json:"node_id"`
	Local            bool                  `json:"local"`
	HasReading       bool                  `json:"has_reading"`
	Status           NodeCapacityStatus    `json:"status"`
	Settings         NodeGuardrailSettings `json:"settings"`
	CleanupSupported bool                  `json:"cleanup_supported"`
	CleanupReason    string                `json:"cleanup_unsupported_reason,omitempty"`
}

// NodeCleanupItem is one candidate in a cleanup class.
type NodeCleanupItem struct {
	Name      string `json:"name"`
	Detail    string `json:"detail,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	Action    string `json:"action"`
}

// NodeCleanupClass groups cleanup candidates; Deletable false means shown only.
type NodeCleanupClass struct {
	Name      string            `json:"name"`
	Deletable bool              `json:"deletable"`
	Bytes     int64             `json:"bytes"`
	Items     []NodeCleanupItem `json:"items"`
}

// NodeCleanupResult is the response of POST /api/v1/nodes/{id}/cleanup.
type NodeCleanupResult struct {
	DryRun  bool `json:"dry_run"`
	Preview struct {
		Classes    []NodeCleanupClass `json:"classes"`
		TotalBytes int64              `json:"total_bytes"`
	} `json:"preview"`
	FreedBytes int64  `json:"freed_bytes"`
	Error      string `json:"error,omitempty"`
}

// NodeCleanupRunResource is one entry of GET /api/v1/nodes/{id}/cleanup-runs.
type NodeCleanupRunResource struct {
	ID         string             `json:"id"`
	StartedAt  time.Time          `json:"started_at"`
	Trigger    string             `json:"trigger"`
	Actor      string             `json:"actor"`
	FreedBytes int64              `json:"freed_bytes"`
	Classes    []NodeCleanupClass `json:"classes"`
	Error      string             `json:"error,omitempty"`
}

// GetNodeCapacity calls GET /api/v1/nodes/{id}/capacity.
func (c *Client) GetNodeCapacity(ctx context.Context, id string) (NodeCapacityResource, error) {
	var out NodeCapacityResource
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/capacity", nil, &out)
	return out, err
}

// UpdateNodeGuardrails calls PUT /api/v1/nodes/{id}/guardrails.
func (c *Client) UpdateNodeGuardrails(ctx context.Context, id string, s NodeGuardrailSettings) (NodeCapacityResource, error) {
	var out NodeCapacityResource
	err := c.do(ctx, http.MethodPut, nodePath(id)+"/guardrails", s, &out)
	return out, err
}

// CleanupNode calls POST /api/v1/nodes/{id}/cleanup: a preview with dryRun, otherwise a confirmed removal.
func (c *Client) CleanupNode(ctx context.Context, id string, dryRun bool) (NodeCleanupResult, error) {
	var out NodeCleanupResult
	body := map[string]bool{"dry_run": dryRun, "confirm": !dryRun}
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/cleanup", body, &out)
	return out, err
}

// ListNodeCleanupRuns calls GET /api/v1/nodes/{id}/cleanup-runs.
func (c *Client) ListNodeCleanupRuns(ctx context.Context, id string) ([]NodeCleanupRunResource, error) {
	var out []NodeCleanupRunResource
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/cleanup-runs", nil, &out)
	return out, err
}
