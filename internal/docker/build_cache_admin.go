package docker

import (
	"context"
	"fmt"
	"sort"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/filters"
)

// BuildCacheRecord is one BuildKit cache record with the fields the build center shows.
type BuildCacheRecord struct {
	ID          string
	Type        string
	Description string
	SizeBytes   int64
	InUse       bool
	Shared      bool
	UsageCount  int
	CreatedAt   time.Time
	LastUsedAt  time.Time
}

// BuildCacheAdmin lists and prunes the BuildKit cache of one node.
type BuildCacheAdmin interface {
	BuildCacheRecords(ctx context.Context) ([]BuildCacheRecord, error)
	PruneBuildCacheOlderThan(ctx context.Context, olderThan time.Duration) (PruneBuildCacheResult, error)
}

// BuildCacheRecords lists every cache record in the daemon's BuildKit instance, largest first.
func (c *Client) BuildCacheRecords(ctx context.Context) ([]BuildCacheRecord, error) {
	du, err := c.cli.DiskUsage(ctx, dockertypes.DiskUsageOptions{})
	if err != nil {
		return nil, fmt.Errorf("docker: build cache records: %w", err)
	}
	out := make([]BuildCacheRecord, 0, len(du.BuildCache))
	for _, r := range du.BuildCache {
		if r == nil {
			continue
		}
		rec := BuildCacheRecord{
			ID: r.ID, Type: r.Type, Description: r.Description, SizeBytes: r.Size,
			InUse: r.InUse, Shared: r.Shared, UsageCount: r.UsageCount, CreatedAt: r.CreatedAt,
		}
		if r.LastUsedAt != nil {
			rec.LastUsedAt = *r.LastUsedAt
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SizeBytes > out[j].SizeBytes })
	return out, nil
}

// PruneBuildCacheOlderThan removes cache records unused for olderThan; records an
// in-flight build holds are never touched.
func (c *Client) PruneBuildCacheOlderThan(ctx context.Context, olderThan time.Duration) (PruneBuildCacheResult, error) {
	report, err := c.cli.BuildCachePrune(ctx, build.CachePruneOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("until", olderThan.String())),
	})
	if err != nil {
		return PruneBuildCacheResult{}, fmt.Errorf("docker: prune build cache: %w", err)
	}
	if report == nil {
		return PruneBuildCacheResult{}, nil
	}
	return PruneBuildCacheResult{Removed: report.CachesDeleted, ReclaimedBytes: report.SpaceReclaimed}, nil
}

// PlanBuildCachePrune picks the records a prune of olderThan would remove: idle
// for at least that long and not in use. olderThan 0 selects every idle record.
func PlanBuildCachePrune(records []BuildCacheRecord, olderThan time.Duration, now time.Time) []BuildCacheRecord {
	var out []BuildCacheRecord
	for _, r := range records {
		if r.InUse {
			continue
		}
		last := r.LastUsedAt
		if last.IsZero() {
			last = r.CreatedAt
		}
		if olderThan > 0 && now.Sub(last) < olderThan {
			continue
		}
		out = append(out, r)
	}
	return out
}
