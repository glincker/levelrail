package demo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// liveAnchor is "now": migrations stamp rows with the wall clock, so a fixed
// past anchor would make those look like future timestamps.
var liveAnchor = time.Now().UTC().Truncate(time.Minute)

type seededStores struct {
	Stores
	dir string
}

func seedStores(t *testing.T, anchor time.Time) seededStores {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	tdb, err := telemetry.Open(ctx, filepath.Join(dir, "telemetry.db"))
	if err != nil {
		t.Fatalf("telemetry.Open() error = %v", err)
	}
	adb, err := alerting.Open(ctx, filepath.Join(dir, "alerting.db"))
	if err != nil {
		t.Fatalf("alerting.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close(); _ = tdb.Close(); _ = adb.Close() })
	st := Stores{DB: db, Telemetry: tdb, Alerting: adb}
	start := time.Now()
	if _, err := SeedAll(ctx, st, Options{Seed: 3, Anchor: anchor}); err != nil {
		t.Fatalf("SeedAll() error = %v", err)
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Errorf("seed took %s, want a few seconds", d)
	}
	return seededStores{Stores: st, dir: dir}
}

func TestSeedLintClean(t *testing.T) {
	// A fresh anchor, not liveAnchor: migrations stamp rows with the wall
	// clock, and on a loaded CI runner the package-init anchor is minutes old.
	// Round, not Truncate: truncating can land 59s in the past, which with the
	// lint slack made late-minute starts flag migration rows as future.
	s := seedStores(t, time.Now().UTC().Round(time.Minute))
	findings, err := Lint(context.Background(), s.Stores, LintOptions{})
	if err != nil {
		t.Fatalf("Lint() error = %v", err)
	}
	for _, f := range findings {
		t.Errorf("finding: %s", f)
	}
}

func TestSeedSizeBudget(t *testing.T) {
	s := seedStores(t, liveAnchor)
	var total int64
	for _, f := range []string{"levelrail.db", "telemetry.db", "alerting.db"} {
		if fi, err := os.Stat(filepath.Join(s.dir, f)); err == nil {
			total += fi.Size()
		}
	}
	if total > 20<<20 {
		t.Errorf("seeded data is %d bytes, want under 20 MiB", total)
	}
}

func TestSeedLintCatchesViolations(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(t *testing.T, s seededStores)
		check  string
	}{
		{"secret", func(t *testing.T, s seededStores) {
			exec(t, s.DB, `UPDATE desired_services SET env = '{"K":"AKIAABCDEFGHIJKLMNOP"}' WHERE name = 'n8n'`)
		}, "secret"},
		{"domain", func(t *testing.T, s seededStores) {
			exec(t, s.DB, `UPDATE desired_services SET image = 'registry.acme-real.io/app:v1' WHERE name = 'n8n'`)
		}, "domain"},
		{"future", func(t *testing.T, s seededStores) {
			exec(t, s.DB, `UPDATE audit_log SET created_at = '2099-01-01T00:00:00Z' WHERE id = 'audit_demo_0001'`)
		}, "future-timestamp"},
		{"orphan", func(t *testing.T, s seededStores) {
			exec(t, s.DB, `PRAGMA foreign_keys = OFF`)
			exec(t, s.DB, `UPDATE deploy_attempts SET service_name = 'ghost' WHERE id = (SELECT id FROM deploy_attempts LIMIT 1)`)
		}, "missing-parent"},
	}
	s := seedStores(t, liveAnchor)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(t, s)
			findings, err := Lint(ctx, s.Stores, LintOptions{})
			if err != nil {
				t.Fatalf("Lint() error = %v", err)
			}
			for _, f := range findings {
				if f.Check == tc.check {
					return
				}
			}
			t.Errorf("no %q finding, got %v", tc.check, findings)
		})
	}
}

func exec(t *testing.T, db *store.DB, q string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func TestSeedStoryMetricsAndLogs(t *testing.T) {
	ctx := context.Background()
	s := seedStores(t, liveAnchor)
	story := storyFor(liveAnchor, false)
	id := "service:" + incidentApp

	p95 := func(at time.Time) float64 {
		var counts []float64
		for i := 0; i < telemetry.LatencyBucketCount; i++ {
			rows, err := s.Telemetry.Query(ctx, id, telemetry.LatencyBucketMetric(i), at, at)
			if err != nil || len(rows) == 0 {
				counts = append(counts, 0)
				continue
			}
			counts = append(counts, rows[0].Value)
		}
		return telemetry.PercentileFromBuckets(counts, 0.95)
	}
	before := p95(story.slowDeploy.Add(-10 * time.Minute))
	during := p95(story.slowDeploy.Add(20 * time.Minute))
	after := p95(story.slowRecovered.Add(10 * time.Minute))
	if during < before*1.8 || after > before*1.2 {
		t.Errorf("p95 before/during/after = %.1f/%.1f/%.1f, want a spike that recovers", before, during, after)
	}

	last, err := s.Telemetry.Query(ctx, id, "cpu_percent", liveAnchor.Add(-time.Minute), liveAnchor)
	if err != nil || len(last) == 0 {
		t.Errorf("no sample at the anchor, err = %v", err)
	}
	deploys, err := s.Telemetry.Query(ctx, id, telemetry.MetricDeployCount, liveAnchor.AddDate(0, 0, -14), liveAnchor)
	if err != nil || len(deploys) == 0 {
		t.Errorf("no deploy markers, err = %v", err)
	}

	lines, err := s.Telemetry.QueryLogs(ctx, "service:"+crashloopApp, liveAnchor.Add(-time.Hour), liveAnchor, "")
	if err != nil || len(lines) < 200 {
		t.Fatalf("crashloop lines = %d, err = %v, want at least 200", len(lines), err)
	}
	structured := 0
	all, _ := s.Telemetry.QueryLogs(ctx, id, liveAnchor.AddDate(0, 0, -4), liveAnchor, "")
	for _, l := range all {
		if l.Structured {
			structured++
		}
	}
	if structured == 0 || structured == len(all) {
		t.Errorf("structured lines = %d of %d, want a mix", structured, len(all))
	}
	if last := lines[len(lines)-1].Message; !strings.Contains(last, "memory") {
		t.Errorf("last crashloop line = %q, want an out of memory line", last)
	}
}

func TestSeedAlertsAndOps(t *testing.T) {
	ctx := context.Background()
	s := seedStores(t, liveAnchor)

	rules, err := s.Alerting.ListRules(ctx)
	if err != nil || len(rules) != 8 {
		t.Fatalf("rules = %d, err = %v, want 8", len(rules), err)
	}
	var firing int
	for _, r := range rules {
		if r.Firing {
			firing++
		}
	}
	if firing != 1 {
		t.Errorf("firing rules = %d, want 1", firing)
	}
	hist, err := s.Alerting.ListHistory(ctx, alerting.HistoryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("ListHistory() error = %v", err)
	}
	var fired, resolved int
	for _, h := range hist {
		switch h.Event {
		case alerting.EventFired:
			fired++
		case alerting.EventResolved:
			resolved++
		}
		if h.At.After(liveAnchor) {
			t.Errorf("history entry %s is after the anchor", h.ID)
		}
	}
	if fired != 14 || resolved != 13 {
		t.Errorf("fired/resolved = %d/%d, want 14/13", fired, resolved)
	}

	incidents, err := s.DB.ListStatusDaily(ctx, "2000-01-01")
	if err != nil || len(incidents) == 0 {
		t.Errorf("status daily = %d, err = %v", len(incidents), err)
	}
	members, err := s.DB.ListTeamMembers(ctx, store.DefaultTeamID)
	if err != nil {
		t.Fatalf("ListTeamMembers() error = %v", err)
	}
	roles := map[string]bool{}
	for _, m := range members {
		roles[m.Role] = true
	}
	if len(roles) < 3 {
		t.Errorf("team roles = %v, want at least 3 distinct", roles)
	}
}
