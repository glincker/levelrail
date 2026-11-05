package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func parkedAgedRows(t *testing.T, rt *Router, db *store.DB, ages map[string]int, now time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	for d, age := range ages {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{d}}); err != nil {
			t.Fatal(err)
		}
		if err := db.ParkServiceDomain(ctx, "web", d, "tester", ""); err != nil {
			t.Fatal(err)
		}
		at := now.Add(-time.Duration(age) * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `UPDATE parked_domains SET parked_at = ? WHERE domain = ?`, at, d); err != nil {
			t.Fatal(err)
		}
	}
	_ = rt
}

func setRetention(t *testing.T, rt *Router, days string) {
	t.Helper()
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyDomainsParkedRetentionDays, json.RawMessage(days), "test"); err != nil {
		t.Fatal(err)
	}
}

func TestParkedRetentionSweep(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	parkedAgedRows(t, rt, db, map[string]int{"old.example.com": 100, "mid.example.com": 40, "new.example.com": 5}, now)

	if n, err := rt.SweepParkedRetention(ctx, now); err != nil || n != 0 {
		t.Fatalf("setting 0 must delete nothing: %d, %v", n, err)
	}
	if rows, _ := db.ListParkedDomains(ctx); len(rows) != 3 {
		t.Fatalf("rows at 0 = %d", len(rows))
	}

	setRetention(t, rt, "30")
	cookie := loginTestSession(t, rt, db)
	rec := sdReq(t, rt, cookie, http.MethodGet, "/api/v1/domains/parked/retention", "")
	var plan parkedRetentionPlan
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &plan) != nil {
		t.Fatalf("dry run = %d %s", rec.Code, rec.Body.String())
	}
	if plan.RetentionDays != 30 || plan.NextRun == nil {
		t.Fatalf("plan header = %+v", plan)
	}
	if rows, _ := db.ListParkedDomains(ctx); len(rows) != 3 {
		t.Fatal("dry run deleted rows")
	}

	// The HTTP dry run uses wall time, so compare against the same planner at the fake clock.
	rows, _ := db.ListParkedDomains(ctx)
	want := planParkedRetention(rows, 30, now).WouldDelete
	if len(want) != 2 || want[0].Domain != "old.example.com" || want[1].Domain != "mid.example.com" {
		t.Fatalf("planned = %+v", want)
	}
	if n, err := rt.SweepParkedRetention(ctx, now); err != nil || n != 2 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	left, _ := db.ListParkedDomains(ctx)
	if len(left) != 1 || left[0].Domain != "new.example.com" {
		t.Fatalf("left = %+v", left)
	}
	if n, err := rt.SweepParkedRetention(ctx, now); err != nil || n != 0 {
		t.Fatalf("second sweep must be a no-op: %d, %v", n, err)
	}

	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, e := range entries {
		if e.Action == auditActionParkedDeleted && e.ActorType == auditActorSystem {
			found++
			var d struct {
				Reason  string `json:"reason"`
				AgeDays int    `json:"age_days"`
			}
			if json.Unmarshal([]byte(e.Detail), &d) != nil || d.Reason != "retention" || d.AgeDays < 40 {
				t.Fatalf("audit detail = %q", e.Detail)
			}
		}
	}
	if found != 2 {
		t.Fatalf("audit entries = %d, want 2", found)
	}
}

func TestParkedRetentionBoundedBatch(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	var rows []store.ParkedDomain
	for i := range parkedRetentionBatch + 7 {
		rows = append(rows, store.ParkedDomain{Domain: string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".example.com", ParkedAt: now.Add(-time.Duration(60+i) * 24 * time.Hour)})
	}
	plan := planParkedRetention(rows, 30, now)
	if len(plan.WouldDelete) != parkedRetentionBatch || plan.TotalEligible != parkedRetentionBatch+7 {
		t.Fatalf("batch = %d eligible = %d", len(plan.WouldDelete), plan.TotalEligible)
	}
	if got := planParkedRetention(rows, 0, now); len(got.WouldDelete) != 0 {
		t.Fatal("0 days must plan nothing")
	}
}
