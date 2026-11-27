package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/imagelife"
	"github.com/GLINCKER/levelrail/internal/janitor"
)

// AuditActionDiskGC is written once per applied disk cleanup.
const AuditActionDiskGC = "disk.gc"

type diskGCRequest struct {
	// Apply removes what the report lists; the default is a dry run.
	Apply bool `json:"apply"`
	// IfNeeded makes an apply a no-op while free space is above the janitor budget.
	IfNeeded bool `json:"if_needed"`
}

type diskGCCategory struct {
	Category       string `json:"category"`
	Count          int    `json:"count"`
	ReclaimBytes   int64  `json:"reclaim_bytes"`
	ReclaimedBytes int64  `json:"reclaimed_bytes"`
	Error          string `json:"error,omitempty"`
}

type diskGCResponse struct {
	DryRun            bool             `json:"dry_run"`
	Skipped           string           `json:"skipped,omitempty"`
	FreePercent       float64          `json:"free_percent"`
	BudgetPercent     float64          `json:"budget_percent"`
	Categories        []diskGCCategory `json:"categories"`
	TotalReclaimBytes int64            `json:"total_reclaim_bytes"`
	TotalReclaimed    int64            `json:"total_reclaimed_bytes"`
}

// planDiskGC picks what a cleanup may remove. Image protection (live, rollback
// pins, newest deploys, in-use) is already decided in entries by imagelife.
func planDiskGC(entries []imagelife.Entry, cache []docker.BuildCacheRecord, cacheAge time.Duration, now time.Time) (images []imagelife.Entry, imageBytes int64, cacheRecs []docker.BuildCacheRecord, cacheBytes int64) {
	plan := imagelife.PlanGC(entries)
	images = retentionSafeRemovals(plan.Remove)
	imageBytes = plan.ReclaimMaxBytes
	cacheRecs = docker.PlanBuildCachePrune(cache, cacheAge, now)
	for _, r := range cacheRecs {
		cacheBytes += r.SizeBytes
	}
	return images, imageBytes, cacheRecs, cacheBytes
}

// handleDiskGC handles POST /api/v1/system/disk/gc for the control plane node.
// Remote nodes wait for the fencing token on destructive node operations.
func (rt *Router) handleDiskGC(w http.ResponseWriter, r *http.Request) {
	if !rt.janitorReady(w) {
		return
	}
	var req diskGCRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	ctx := r.Context()
	inv, ok := rt.imageInventoryFor(w, "")
	if !ok {
		return
	}
	pol, err := janitor.LoadPolicy(ctx, rt.janitorStore)
	if err != nil {
		rt.internalError(w, "api: disk gc policy failed", err)
		return
	}
	resp := diskGCResponse{DryRun: !req.Apply, BudgetPercent: pol.DiskFreeBudgetPercent, Categories: []diskGCCategory{}}
	if free, total, derr := rt.fixitDiskFree(ctx); derr == nil && total > 0 {
		resp.FreePercent = float64(free) / float64(total) * 100
	}
	entries, err := rt.buildImageEntries(ctx, inv, "", rt.imageRetentionDefaults())
	if err != nil {
		rt.internalError(w, "api: disk gc image plan failed", err)
		return
	}
	cache, err := rt.janitor.Docker.BuildCacheRecords(ctx)
	if err != nil {
		rt.internalError(w, "api: disk gc cache plan failed", err)
		return
	}
	cacheAge := time.Duration(pol.BuildCacheMaxAgeHours) * time.Hour
	images, imageBytes, cacheRecs, cacheBytes := planDiskGC(entries, cache, cacheAge, time.Now())
	imgCat := diskGCCategory{Category: "images", Count: len(images), ReclaimBytes: imageBytes}
	cacheCat := diskGCCategory{Category: "build_cache", Count: len(cacheRecs), ReclaimBytes: cacheBytes}
	switch {
	case req.Apply && req.IfNeeded && resp.FreePercent >= pol.DiskFreeBudgetPercent:
		resp.Skipped = "free space is above the budget"
		resp.DryRun = true
	case req.Apply:
		for _, e := range images {
			if !rt.removalStillSafe(ctx, e) {
				continue
			}
			if _, rerr := removeImageEntry(ctx, inv, e); rerr != nil {
				imgCat.Error = rerr.Error()
				continue
			}
			imgCat.ReclaimedBytes += e.SizeBytes - e.SharedBytes
		}
		if len(cacheRecs) > 0 {
			pr, perr := rt.janitor.Docker.PruneBuildCacheOlderThan(ctx, cacheAge)
			if perr != nil {
				cacheCat.Error = perr.Error()
			}
			cacheCat.ReclaimedBytes = int64(pr.ReclaimedBytes) //nolint:gosec // docker reports bytes well below int64 range
		}
		resp.TotalReclaimed = imgCat.ReclaimedBytes + cacheCat.ReclaimedBytes
		rt.logger.Info("api: disk gc finished", slog.Int64("reclaimed", resp.TotalReclaimed))
		rt.auditImageRetention(r, AuditActionDiskGC, "", map[string]any{"images": imgCat.Count, "cache_records": cacheCat.Count, "reclaimed_bytes": resp.TotalReclaimed})
	}
	resp.Categories = append(resp.Categories, imgCat, cacheCat)
	resp.TotalReclaimBytes = imageBytes + cacheBytes
	writeJSON(w, http.StatusOK, resp)
}
