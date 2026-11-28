package apiclient

import (
	"context"
	"net/http"

	"github.com/glincker/levelrail/internal/diskguard"
)

// DiskResilience mirrors GET /api/v1/resilience/disk.
type DiskResilience struct {
	diskguard.Status
	Refusing  bool `json:"refusing"`
	FixAction bool `json:"free_space_available"`
}

// FreeSpaceResult mirrors POST /api/v1/resilience/disk/free-space.
type FreeSpaceResult struct {
	ReclaimedBytes int64          `json:"reclaimed_bytes"`
	Disk           DiskResilience `json:"disk"`
}

// GetDiskResilience calls GET /api/v1/resilience/disk.
func (c *Client) GetDiskResilience(ctx context.Context) (DiskResilience, error) {
	return getJSON[DiskResilience](ctx, c, "/api/v1/resilience/disk")
}

// FreeDiskSpace calls POST /api/v1/resilience/disk/free-space.
func (c *Client) FreeDiskSpace(ctx context.Context) (FreeSpaceResult, error) {
	var out FreeSpaceResult
	err := c.do(ctx, http.MethodPost, "/api/v1/resilience/disk/free-space", nil, &out)
	return out, err
}
