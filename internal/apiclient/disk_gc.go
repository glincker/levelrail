package apiclient

import (
	"context"
	"net/http"
)

// DiskGCRequest is the body of POST /api/v1/system/disk/gc.
type DiskGCRequest struct {
	Apply    bool `json:"apply"`
	IfNeeded bool `json:"if_needed"`
}

// DiskGCCategory is one row of the reclaimable report.
type DiskGCCategory struct {
	Category       string `json:"category"`
	Count          int    `json:"count"`
	ReclaimBytes   int64  `json:"reclaim_bytes"`
	ReclaimedBytes int64  `json:"reclaimed_bytes"`
	Error          string `json:"error,omitempty"`
}

// DiskGCReport is the dry run or apply result.
type DiskGCReport struct {
	DryRun            bool             `json:"dry_run"`
	Skipped           string           `json:"skipped,omitempty"`
	FreePercent       float64          `json:"free_percent"`
	BudgetPercent     float64          `json:"budget_percent"`
	Categories        []DiskGCCategory `json:"categories"`
	TotalReclaimBytes int64            `json:"total_reclaim_bytes"`
	TotalReclaimed    int64            `json:"total_reclaimed_bytes"`
}

// RunDiskGC calls POST /api/v1/system/disk/gc.
func (c *Client) RunDiskGC(ctx context.Context, req DiskGCRequest) (DiskGCReport, error) {
	var out DiskGCReport
	err := c.do(ctx, http.MethodPost, "/api/v1/system/disk/gc", req, &out)
	return out, err
}
