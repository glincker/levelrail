package apiclient

import (
	"context"
	"net/http"
	"time"
)

// NodeFailureAction is one resource of a failed node and what happens to it.
type NodeFailureAction struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Class  string `json:"class"`
	Status string `json:"status"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// NodeFailurePolicy is a node's stored override; empty mode and zero grace mean the default.
type NodeFailurePolicy struct {
	NodeID       string `json:"node_id"`
	Mode         string `json:"mode"`
	GraceSeconds int    `json:"grace_seconds"`
}

// NodeFailureStatus mirrors GET /api/v1/nodes/{id}/failure.
type NodeFailureStatus struct {
	NodeID                string              `json:"node_id"`
	Mode                  string              `json:"mode"`
	GraceSeconds          int                 `json:"grace_seconds"`
	Phase                 string              `json:"phase"`
	SilentSeconds         int                 `json:"silent_seconds"`
	GraceRemainingSeconds int                 `json:"grace_remaining_seconds"`
	Reason                string              `json:"reason,omitempty"`
	UnreachableAt         *time.Time          `json:"unreachable_at,omitempty"`
	Consequences          []NodeFailureAction `json:"consequences"`
	Policy                NodeFailurePolicy   `json:"policy"`
}

// NodeFailureSummary is the compact failure view embedded in a node resource.
type NodeFailureSummary struct {
	Phase    string `json:"phase"`
	Reason   string `json:"reason,omitempty"`
	NeedsYou int    `json:"needs_decision"`
	Moved    int    `json:"moved"`
	Since    string `json:"unreachable_at,omitempty"`
}

// GetNodeFailure calls GET /api/v1/nodes/{id}/failure.
func (c *Client) GetNodeFailure(ctx context.Context, id string) (NodeFailureStatus, error) {
	var out NodeFailureStatus
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/failure", nil, &out)
	return out, err
}

// SetNodeFailurePolicy calls PUT /api/v1/nodes/{id}/failure-policy.
func (c *Client) SetNodeFailurePolicy(ctx context.Context, id, mode string, graceSeconds int) (NodeFailureStatus, error) {
	var out NodeFailureStatus
	body := map[string]any{"mode": mode, "grace_seconds": graceSeconds}
	err := c.do(ctx, http.MethodPut, nodePath(id)+"/failure-policy", body, &out)
	return out, err
}

// MarkNodeLost calls POST /api/v1/nodes/{id}/failure/lost; dryRun returns the plan without acting.
func (c *Client) MarkNodeLost(ctx context.Context, id string, dryRun bool) (NodeFailureStatus, error) {
	var out NodeFailureStatus
	body := map[string]any{"dry_run": dryRun, "confirm": !dryRun}
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/failure/lost", body, &out)
	return out, err
}

// ResolveNodeFailure calls POST /api/v1/nodes/{id}/failure/resolve with action wait or restore.
func (c *Client) ResolveNodeFailure(ctx context.Context, id, kind, name, action, target string) (NodeFailureStatus, error) {
	var out NodeFailureStatus
	body := map[string]any{"kind": kind, "name": name, "action": action, "target_node_id": target}
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/failure/resolve", body, &out)
	return out, err
}
