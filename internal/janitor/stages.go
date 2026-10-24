package janitor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const noPressure = "free space is above the budget"

func skipped(s Stage, reason string) StageResult {
	return StageResult{Stage: s, Status: StatusSkipped, Reason: reason, Items: []Item{}}
}

func failed(s Stage, err error) StageResult {
	return StageResult{Stage: s, Status: StatusFailed, Error: err.Error(), Items: []Item{}}
}

func u64(v uint64) int64 {
	const maxInt64 = 1<<63 - 1
	if v > maxInt64 {
		return maxInt64
	}
	return int64(v)
}

// containers removes stopped containers no desired state still owns.
func (e *Engine) containers(ctx context.Context, dry, pressure bool) StageResult {
	if !pressure {
		return skipped(StageContainers, noPressure)
	}
	if dry {
		u, err := e.Docker.DiskUsage(ctx)
		if err != nil {
			return failed(StageContainers, err)
		}
		res := StageResult{Stage: StageContainers, Status: StatusRan, Items: []Item{}}
		if u.ContainersReclaimableBytes > 0 {
			res.Items = append(res.Items, Item{Name: "stopped containers", Detail: "upper bound, apps still wanted are kept", SizeBytes: u.ContainersReclaimableBytes, Action: ActionWouldRemove})
		}
		return res
	}
	keep, err := e.DesiredContainers(ctx)
	if err != nil {
		return failed(StageContainers, err)
	}
	pr, err := e.Docker.PruneContainers(ctx, keep)
	res := StageResult{Stage: StageContainers, Status: StatusRan, ReclaimedBytes: u64(pr.ReclaimedBytes), Items: []Item{}}
	for _, n := range pr.Removed {
		res.Items = append(res.Items, Item{Name: n, Action: ActionRemoved})
	}
	if err != nil {
		res.Status, res.Error = StatusFailed, err.Error()
	}
	return res
}

// images removes dangling images only. Tagged images, which include every
// rollback target, are never candidates.
func (e *Engine) images(ctx context.Context, dry, pressure bool) StageResult {
	if !pressure {
		return skipped(StageImages, noPressure)
	}
	if dry {
		recs, err := e.Docker.ListAllImages(ctx)
		if err != nil {
			return failed(StageImages, err)
		}
		res := StageResult{Stage: StageImages, Status: StatusRan, Items: []Item{}}
		for _, r := range recs {
			if len(r.Tags) == 0 && r.Containers == 0 {
				res.Items = append(res.Items, Item{Name: shortID(r.ID), SizeBytes: r.SizeBytes, Action: ActionWouldRemove})
			}
		}
		return res
	}
	pr, err := e.Docker.PruneDanglingImages(ctx)
	if err != nil {
		return failed(StageImages, err)
	}
	res := StageResult{Stage: StageImages, Status: StatusRan, ReclaimedBytes: u64(pr.ReclaimedBytes), Items: []Item{}}
	for _, n := range pr.Removed {
		res.Items = append(res.Items, Item{Name: shortID(n), Action: ActionRemoved})
	}
	return res
}

func shortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// buildCache always trims idle records past the age limit, and trims every
// idle record when space is short or the cache is over its budget.
func (e *Engine) buildCache(ctx context.Context, p Policy, dry, pressure bool, now time.Time) StageResult {
	recs, err := e.Docker.BuildCacheRecords(ctx)
	if err != nil {
		return failed(StageBuildCache, err)
	}
	var total int64
	for _, r := range recs {
		total += r.SizeBytes
	}
	age := time.Duration(p.BuildCacheMaxAgeHours) * time.Hour
	reason := "idle past the age limit"
	if pressure || (p.BuildCacheBudgetMB > 0 && total > p.BuildCacheBudgetMB<<20) {
		age = time.Hour
		reason = "cache over budget or disk short"
	}
	res := StageResult{Stage: StageBuildCache, Status: StatusRan, Reason: reason, Items: []Item{}}
	plan := docker.PlanBuildCachePrune(recs, age, now)
	if dry {
		var sum int64
		for _, r := range plan {
			sum += r.SizeBytes
		}
		if len(plan) > 0 {
			res.Items = append(res.Items, Item{Name: fmt.Sprintf("%d cache records", len(plan)), SizeBytes: sum, Action: ActionWouldRemove})
		}
		return res
	}
	if len(plan) == 0 {
		return res
	}
	pr, err := e.Docker.PruneBuildCacheOlderThan(ctx, age)
	if err != nil {
		return failed(StageBuildCache, err)
	}
	res.ReclaimedBytes = u64(pr.ReclaimedBytes)
	if len(pr.Removed) > 0 {
		res.Items = append(res.Items, Item{Name: fmt.Sprintf("%d cache records", len(pr.Removed)), SizeBytes: res.ReclaimedBytes, Action: ActionRemoved})
	}
	return res
}

// volumes quarantines orphaned named volumes, then deletes one only after the
// quarantine ended and the operator opted in. Database volumes and anything
// still mounted are never touched.
func (e *Engine) volumes(ctx context.Context, p Policy, dry, pressure bool, now time.Time) StageResult {
	res := StageResult{Stage: StageVolumes, Status: StatusRan, Items: []Item{}}
	if e.OrphanVolumes == nil {
		return skipped(StageVolumes, "volume management is not configured")
	}
	orphans, err := e.OrphanVolumes(ctx)
	if err != nil {
		return failed(StageVolumes, err)
	}
	held, err := e.Store.ListJanitorQuarantine(ctx, volumeKind)
	if err != nil {
		return failed(StageVolumes, err)
	}
	since := make(map[string]time.Time, len(held))
	for _, q := range held {
		since[q.Name] = q.FirstSeen
	}
	current := make(map[string]bool, len(orphans))
	quarantine := time.Duration(p.VolumeQuarantineDays) * 24 * time.Hour
	removals := 0
	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Name < orphans[j].Name })
	for _, v := range orphans {
		current[v.Name] = true
		size := v.SizeBytes
		if v.Mounted {
			continue
		}
		if strings.HasPrefix(v.Name, "db-") {
			res.Items = append(res.Items, Item{Name: v.Name, Detail: "database data is never removed automatically", SizeBytes: size, Action: ActionKept})
			continue
		}
		first, quarantined := since[v.Name]
		switch {
		case !quarantined:
			act := ActionWouldQuarantine
			if !dry {
				if err := e.Store.UpsertJanitorQuarantine(ctx, store.JanitorQuarantine{Kind: volumeKind, Name: v.Name, SizeBytes: size, FirstSeen: now}); err != nil {
					return failed(StageVolumes, err)
				}
				act = ActionQuarantined
			}
			res.Items = append(res.Items, Item{Name: v.Name, Detail: fmt.Sprintf("held for %d days before it may be deleted", p.VolumeQuarantineDays), SizeBytes: size, Action: act})
		case now.Sub(first) < quarantine:
			left := quarantine - now.Sub(first)
			res.Items = append(res.Items, Item{Name: v.Name, Detail: fmt.Sprintf("quarantined, %d days left", int(left.Hours()/24)+1), SizeBytes: size, Action: ActionKept})
		case !p.DeleteOrphanVolumes:
			res.Items = append(res.Items, Item{Name: v.Name, Detail: "quarantine over, automatic deletion is off", SizeBytes: size, Action: ActionKept})
		case removals >= maxVolumeRemovalsPerRun:
			res.Items = append(res.Items, Item{Name: v.Name, Detail: "deferred to the next pass", SizeBytes: size, Action: ActionKept})
		case dry:
			removals++
			res.Items = append(res.Items, Item{Name: v.Name, SizeBytes: size, Action: ActionWouldRemove})
		default:
			removals++
			if err := e.RemoveVolume(ctx, v.Name); err != nil {
				res.Status, res.Error = StatusFailed, err.Error()
				continue
			}
			_ = e.Store.DeleteJanitorQuarantine(ctx, volumeKind, v.Name)
			if size > 0 {
				res.ReclaimedBytes += size
			}
			res.Items = append(res.Items, Item{Name: v.Name, SizeBytes: size, Action: ActionRemoved})
		}
	}
	if !dry {
		for _, q := range held {
			if !current[q.Name] {
				_ = e.Store.DeleteJanitorQuarantine(ctx, volumeKind, q.Name)
			}
		}
	}
	if pressure && !dry {
		pr, err := e.Docker.PruneAnonymousVolumes(ctx)
		if err != nil {
			res.Status, res.Error = StatusFailed, err.Error()
		} else if len(pr.Removed) > 0 {
			res.ReclaimedBytes += u64(pr.ReclaimedBytes)
			res.Items = append(res.Items, Item{Name: fmt.Sprintf("%d anonymous volumes", len(pr.Removed)), SizeBytes: u64(pr.ReclaimedBytes), Action: ActionRemoved})
		}
	}
	return res
}

// attempts deletes old finished deploy attempts that never succeeded. Each
// app's newest attempts, succeeded attempts and pinned rollback targets stay.
func (e *Engine) attempts(ctx context.Context, p Policy, dry bool, now time.Time) StageResult {
	cutoff := now.Add(-time.Duration(p.AttemptRetentionDays) * 24 * time.Hour)
	list, err := e.Store.ListPrunableDeployAttempts(ctx, cutoff, p.AttemptKeepPerApp, maxAttemptsPerPass)
	if err != nil {
		return failed(StageAttempts, err)
	}
	res := StageResult{Stage: StageAttempts, Status: StatusRan, Items: []Item{}}
	if len(list) == 0 {
		return res
	}
	perApp := map[string]int{}
	ids := make([]string, 0, len(list))
	for _, a := range list {
		perApp[a.ServiceName]++
		ids = append(ids, a.ID)
	}
	act := ActionWouldRemove
	if !dry {
		if err := e.Store.DeleteDeployAttempts(ctx, ids); err != nil {
			return failed(StageAttempts, err)
		}
		act = ActionRemoved
	}
	names := make([]string, 0, len(perApp))
	for n := range perApp {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		res.Items = append(res.Items, Item{Name: n, Detail: fmt.Sprintf("%d old failed or superseded attempts", perApp[n]), Count: perApp[n], Action: act})
	}
	return res
}
