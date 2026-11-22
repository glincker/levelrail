package build

import (
	"context"
	"fmt"
	"sort"
	"time"

	bkclient "github.com/moby/buildkit/client"
)

// AppCacheReclaim reports one quota reclaim of an app's cache mounts.
type AppCacheReclaim struct {
	UsageBytes   int64
	QuotaBytes   int64
	FreedBytes   int64
	Removed      int
	SkippedInUse int
	// Bounded means the per run record limit stopped the reclaim before the app fit.
	Bounded bool
}

// PlanAppCacheQuota picks the least recently used records to remove so recs fit
// quotaBytes. In use records, and records used after snapshot (a build touched
// them during the scan), are skipped and counted; quota <= 0 means off.
func PlanAppCacheQuota(recs []*bkclient.UsageInfo, quotaBytes int64, maxRecords int, snapshot time.Time) ([]*bkclient.UsageInfo, AppCacheReclaim) {
	plan := AppCacheReclaim{QuotaBytes: quotaBytes}
	for _, r := range recs {
		plan.UsageBytes += r.Size
	}
	if quotaBytes <= 0 || plan.UsageBytes <= quotaBytes {
		return nil, plan
	}
	sorted := append([]*bkclient.UsageInfo(nil), recs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := lastUsed(sorted[i]), lastUsed(sorted[j])
		if !a.Equal(b) {
			return a.Before(b)
		}
		return sorted[i].ID < sorted[j].ID
	})
	left := plan.UsageBytes
	var victims []*bkclient.UsageInfo
	for _, r := range sorted {
		if left <= quotaBytes {
			break
		}
		if r.InUse || (!snapshot.IsZero() && lastUsed(r).After(snapshot)) {
			plan.SkippedInUse++
			continue
		}
		if maxRecords > 0 && len(victims) >= maxRecords {
			plan.Bounded = true
			break
		}
		victims = append(victims, r)
		left -= r.Size
	}
	return victims, plan
}

func lastUsed(r *bkclient.UsageInfo) time.Time {
	if r.LastUsedAt != nil {
		return *r.LastUsedAt
	}
	return r.CreatedAt
}

// ReclaimAppCacheQuota removes cacheID's least recently used cache mounts until
// it fits quotaBytes, by record id only, so other apps and the shared cache stay.
func (c *Client) ReclaimAppCacheQuota(ctx context.Context, cacheID string, siblings []string, quotaBytes int64, maxRecords int) (AppCacheReclaim, error) {
	if c == nil || c.bk == nil {
		return AppCacheReclaim{}, ErrAppCacheNoBuilder
	}
	snapshot := time.Now()
	du, err := c.bk.DiskUsage(ctx)
	if err != nil {
		return AppCacheReclaim{}, fmt.Errorf("build: disk usage: %w", err)
	}
	victims, plan := PlanAppCacheQuota(AppCacheRecords(du, cacheID, siblings), quotaBytes, maxRecords, snapshot)
	return c.pruneVictims(ctx, victims, plan)
}

// ReclaimNodeDependencyCache trims every dependency cache mount on this builder
// (npm, pip, Go modules and friends, whichever app owns them) to capBytes, least
// recently used first. It backs the per node cap on top of the per app quota.
func (c *Client) ReclaimNodeDependencyCache(ctx context.Context, capBytes int64, maxRecords int) (AppCacheReclaim, error) {
	if c == nil || c.bk == nil {
		return AppCacheReclaim{}, ErrAppCacheNoBuilder
	}
	snapshot := time.Now()
	du, err := c.bk.DiskUsage(ctx)
	if err != nil {
		return AppCacheReclaim{}, fmt.Errorf("build: disk usage: %w", err)
	}
	var mounts []*bkclient.UsageInfo
	for _, r := range du {
		if r != nil && string(r.RecordType) == cacheMountRecordType {
			mounts = append(mounts, r)
		}
	}
	victims, plan := PlanAppCacheQuota(mounts, capBytes, maxRecords, snapshot)
	return c.pruneVictims(ctx, victims, plan)
}

func (c *Client) pruneVictims(ctx context.Context, victims []*bkclient.UsageInfo, plan AppCacheReclaim) (AppCacheReclaim, error) {
	if len(victims) == 0 {
		return plan, nil
	}
	filters := make([]string, 0, len(victims))
	for _, r := range victims {
		filters = append(filters, "id=="+r.ID)
	}
	ch := make(chan bkclient.UsageInfo)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for u := range ch {
			plan.FreedBytes += u.Size
			plan.Removed++
		}
	}()
	err := c.bk.Prune(ctx, ch, bkclient.WithFilter(filters), bkclient.PruneAll)
	close(ch)
	<-done
	if err != nil {
		return plan, fmt.Errorf("build: reclaim app cache: %w", err)
	}
	return plan, nil
}
