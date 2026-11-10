package build

import (
	"testing"
	"time"

	bkclient "github.com/moby/buildkit/client"
)

func quotaRec(id string, size int64, ageHours int, inUse bool) *bkclient.UsageInfo {
	t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(ageHours) * time.Hour)
	return &bkclient.UsageInfo{ID: id, Size: size, LastUsedAt: &t, InUse: inUse, RecordType: cacheMountRecordType}
}

func TestCacheQuotaRemovesOldestFirst(t *testing.T) {
	recs := []*bkclient.UsageInfo{quotaRec("new", 40, 1, false), quotaRec("old", 30, 48, false), quotaRec("mid", 30, 10, false)}
	victims, plan := PlanAppCacheQuota(recs, 60, 10, time.Time{})
	if len(victims) != 2 || victims[0].ID != "old" || victims[1].ID != "mid" {
		t.Fatalf("victims = %v, want old then mid", quotaIDs(victims))
	}
	if plan.UsageBytes != 100 || plan.Bounded || plan.SkippedInUse != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestCacheQuotaSkipsInUseAndReports(t *testing.T) {
	recs := []*bkclient.UsageInfo{quotaRec("busy", 50, 99, true), quotaRec("idle", 50, 5, false)}
	victims, plan := PlanAppCacheQuota(recs, 60, 10, time.Time{})
	if len(victims) != 1 || victims[0].ID != "idle" {
		t.Fatalf("victims = %v, want only idle", quotaIDs(victims))
	}
	if plan.SkippedInUse != 1 {
		t.Fatalf("skipped = %d, want 1", plan.SkippedInUse)
	}
}

func TestCacheQuotaOffAndBounded(t *testing.T) {
	recs := []*bkclient.UsageInfo{quotaRec("a", 50, 3, false), quotaRec("b", 50, 2, false), quotaRec("c", 50, 1, false)}
	if v, _ := PlanAppCacheQuota(recs, 0, 10, time.Time{}); len(v) != 0 {
		t.Fatalf("quota 0 must be off, got %v", quotaIDs(v))
	}
	v, plan := PlanAppCacheQuota(recs, 10, 1, time.Time{})
	if len(v) != 1 || v[0].ID != "a" || !plan.Bounded {
		t.Fatalf("bounded run = %v %+v", quotaIDs(v), plan)
	}
}

func quotaIDs(recs []*bkclient.UsageInfo) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

func TestCacheQuotaSkipsRecordsUsedAfterSnapshot(t *testing.T) {
	snap := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(-2 * time.Hour)
	recs := []*bkclient.UsageInfo{quotaRec("fresh", 50, 1, false), quotaRec("stale", 50, 9, false)}
	victims, plan := PlanAppCacheQuota(recs, 10, 10, snap)
	if len(victims) != 1 || victims[0].ID != "stale" || plan.SkippedInUse != 1 {
		t.Fatalf("victims = %v %+v, want only stale", quotaIDs(victims), plan)
	}
}
