package apiclient

import (
	"context"
	"net/http"
	"time"
)

// PlacementDimension is one resource on one node. A nil Allocatable means
// the node never reported it.
type PlacementDimension struct {
	Name        string `json:"name"`
	Allocatable *int64 `json:"allocatable"`
	Requested   int64  `json:"requested"`
	Need        int64  `json:"need"`
	Free        *int64 `json:"free"`
	Shortfall   int64  `json:"shortfall"`
}

// PlacementEvaluation is how one node fared in a placement decision.
type PlacementEvaluation struct {
	NodeID     string               `json:"node_id"`
	Name       string               `json:"name"`
	Eligible   bool                 `json:"eligible"`
	Reason     string               `json:"reason,omitempty"`
	Capacity   bool                 `json:"capacity_blocked,omitempty"`
	Unverified bool                 `json:"unverified,omitempty"`
	Headroom   *int                 `json:"headroom_permille,omitempty"`
	Load       int                  `json:"load"`
	Dimensions []PlacementDimension `json:"dimensions"`
}

// PlacementPolicy is the operator's overcommit and headroom setting.
type PlacementPolicy struct {
	CPUOvercommitPercent    int  `json:"cpu_overcommit_percent"`
	MemoryOvercommitPercent int  `json:"memory_overcommit_percent"`
	MinFreeDiskPercent      int  `json:"min_free_disk_percent"`
	RejectUnknown           bool `json:"reject_unknown"`
}

// PlacementNeeds is what a workload requests.
type PlacementNeeds struct {
	CPUMilli    int64 `json:"cpu_milli"`
	MemoryBytes int64 `json:"memory_bytes"`
	DiskBytes   int64 `json:"disk_bytes"`
}

// PlacementDecision is the account of one placement choice.
type PlacementDecision struct {
	NodeID     string                `json:"node_id"`
	Why        string                `json:"why"`
	Needs      PlacementNeeds        `json:"needs"`
	Policy     PlacementPolicy       `json:"policy"`
	Considered []PlacementEvaluation `json:"considered"`
	Warnings   []string              `json:"warnings,omitempty"`
}

// RecordedPlacement is a stored decision and what triggered it.
type RecordedPlacement struct {
	Trigger   string            `json:"trigger"`
	DecidedAt time.Time         `json:"decided_at"`
	Decision  PlacementDecision `json:"decision"`
}

// PlacementDecisionView is the recorded decision plus a fresh evaluation.
type PlacementDecisionView struct {
	Kind     string             `json:"kind"`
	Name     string             `json:"name"`
	NodeID   string             `json:"node_id"`
	Recorded *RecordedPlacement `json:"recorded"`
	Preview  PlacementDecision  `json:"preview"`
}

// NodeAllocation is one node's allocatable capacity against its requested totals.
type NodeAllocation struct {
	NodeID      string               `json:"node_id"`
	Name        string               `json:"name"`
	IsLocal     bool                 `json:"is_local"`
	Schedulable bool                 `json:"schedulable"`
	Online      bool                 `json:"online"`
	Workloads   int                  `json:"workloads"`
	Dimensions  []PlacementDimension `json:"dimensions"`
}

// NodeAllocations is every node's allocation plus the policy applied.
type NodeAllocations struct {
	Policy PlacementPolicy  `json:"policy"`
	Nodes  []NodeAllocation `json:"nodes"`
}

// GetPlacementDecision calls GET /api/v1/{apps|databases}/{name}/placement/decision.
func (c *Client) GetPlacementDecision(ctx context.Context, kind, name string) (PlacementDecisionView, error) {
	var out PlacementDecisionView
	err := c.do(ctx, http.MethodGet, movePath(kind, name)+"/placement/decision", nil, &out)
	return out, err
}

// GetNodeAllocations calls GET /api/v1/nodes/allocations.
func (c *Client) GetNodeAllocations(ctx context.Context) (NodeAllocations, error) {
	var out NodeAllocations
	err := c.do(ctx, http.MethodGet, "/api/v1/nodes/allocations", nil, &out)
	return out, err
}
