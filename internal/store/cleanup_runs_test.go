package store

import (
	"testing"
	"time"
)

func TestCleanupRuns_InsertListFilterPrune(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	now := time.Now().UTC()
	runs := []CleanupRun{
		{Target: "a", StartedAt: now.Add(-400 * 24 * time.Hour), FinishedAt: now, Days: 30, Rows: 5, Trigger: "scheduled"},
		{Target: "a", StartedAt: now.Add(-2 * time.Hour), FinishedAt: now, Days: 30, DryRun: true, Rows: 7, Trigger: "manual", Actor: "root"},
		{Target: "b", StartedAt: now.Add(-time.Hour), FinishedAt: now, Days: 90, Rows: 1, Trigger: "api", Error: "boom"},
	}
	for _, r := range runs {
		if _, err := db.InsertCleanupRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	yes := true
	tests := []struct {
		name string
		f    CleanupRunFilter
		want int
	}{
		{"all", CleanupRunFilter{}, 3},
		{"target", CleanupRunFilter{Target: "a"}, 2},
		{"since", CleanupRunFilter{Since: now.Add(-24 * time.Hour)}, 2},
		{"dry", CleanupRunFilter{DryRun: &yes}, 1},
		{"failed", CleanupRunFilter{Status: "failed"}, 1},
		{"ok", CleanupRunFilter{Status: "ok"}, 2},
		{"limit", CleanupRunFilter{Limit: 1}, 1},
	}
	for _, tc := range tests {
		got, err := db.ListCleanupRuns(ctx, tc.f)
		if err != nil || len(got) != tc.want {
			t.Errorf("%s: %d rows, %v, want %d", tc.name, len(got), err, tc.want)
		}
	}

	page1, _ := db.ListCleanupRuns(ctx, CleanupRunFilter{Limit: 2})
	page2, _ := db.ListCleanupRuns(ctx, CleanupRunFilter{Limit: 2, BeforeID: page1[1].ID})
	if len(page2) != 1 || page2[0].Target != "a" || page2[0].ID >= page1[1].ID {
		t.Fatalf("second page = %+v", page2)
	}

	latest, err := db.LatestCleanupRuns(ctx)
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest = %+v, %v", latest, err)
	}
	if failed, _ := db.CleanupFailedSince(ctx, "b", now.Add(-24*time.Hour)); !failed {
		t.Error("failure not seen")
	}
	if failed, _ := db.CleanupFailedSince(ctx, "a", now.Add(-24*time.Hour)); failed {
		t.Error("target a never failed")
	}

	n, err := db.PruneCleanupRuns(ctx, now.Add(-365*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune = %d, %v", n, err)
	}
}

func TestCleanupStatsMatchTieredDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	now := time.Now().UTC()
	for i, age := range []time.Duration{200 * 24 * time.Hour, 100 * 24 * time.Hour, time.Hour} {
		e := AuditEntry{ID: "aud_" + string(rune('a'+i)), ActorType: "system", Method: "GET", Path: "/x", StatusCode: 200,
			CreatedAt: FormatAuditTime(now.Add(-age)), ClientKind: "system", Action: "sensitive_read"}
		if i == 1 {
			e.Action = ""
		}
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	st, err := db.CleanupStats(ctx, CleanupAuditLog)
	if err != nil || st.Rows != 3 || st.Oldest.IsZero() {
		t.Fatalf("stats = %+v, %v", st, err)
	}
	cut, secCut := now.Add(-90*24*time.Hour), now.Add(-365*24*time.Hour)
	want, err := db.CountAuditEntriesTiered(ctx, cut, secCut)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.DeleteAuditEntriesTiered(ctx, cut, secCut)
	if err != nil || got != want || got != 1 {
		t.Fatalf("delete removed %d, count said %d, err %v, want 1 (security row inside its window stays)", got, want, err)
	}
	if n, _ := db.CleanupCountOlder(ctx, CleanupAuditLog, now); n != 2 {
		t.Fatalf("remaining older than now = %d", n)
	}
	if _, err := db.CleanupStats(ctx, CleanupTable("users")); err == nil {
		t.Fatal("an unlisted table was measured")
	}
}
