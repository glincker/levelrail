package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fanoutBackups struct {
	dbs     []store.DesiredDatabase
	history map[string][]store.BackupHistory
}

func (f fanoutBackups) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return f.dbs, nil
}

func (f fanoutBackups) ListBackupHistory(_ context.Context, name string, limit int, _ *time.Time) ([]store.BackupHistory, error) {
	h := f.history[name]
	if len(h) > limit {
		h = h[:limit]
	}
	return h, nil
}

type liveByName map[string]bool

func (liveByName) DatabaseConnections(context.Context, string) (int, int, error) { return 0, 0, nil }
func (liveByName) DatabaseDiskUsedPercent(context.Context, string) (float64, error) {
	return 0, ErrDatabaseHealthUnavailable
}
func (l liveByName) DatabaseUp(_ context.Context, name string) (bool, error) {
	up, ok := l[name]
	if !ok {
		return false, ErrDatabaseHealthUnavailable
	}
	return up, nil
}

func TestDatabaseRulesFanOutAcrossAllDatabases(t *testing.T) {
	now := time.Now()
	ts := func(d time.Duration) string { return now.Add(-d).UTC().Format(time.RFC3339) }
	src := fanoutBackups{
		dbs: []store.DesiredDatabase{
			{Name: "pg", BackupSchedule: "0 3 * * *"},
			{Name: "redis", BackupSchedule: "0 3 * * *"},
			{Name: "old", BackupSchedule: ""},
			{Name: "paused", BackupSchedule: "0 3 * * *", Suspended: true},
		},
		history: map[string][]store.BackupHistory{
			"pg":     {{Status: store.BackupStatusFailed, FinishedAt: ts(time.Hour)}, {Status: store.BackupStatusSucceeded, FinishedAt: ts(72 * time.Hour)}},
			"redis":  {{Status: store.BackupStatusSucceeded, FinishedAt: ts(time.Hour)}},
			"old":    {{Status: store.BackupStatusFailed, FinishedAt: ts(time.Hour)}},
			"paused": {{Status: store.BackupStatusFailed, FinishedAt: ts(time.Hour)}},
		},
	}
	live := liveByName{"pg": true, "redis": false, "paused": false}

	cases := []struct {
		name     string
		kind     Kind
		wantFire bool
		contains string
		excludes string
	}{
		{"down names only the stopped db", KindDatabaseDown, true, "redis", "pg:"},
		{"backup_failed fires for scheduled failing db", KindBackupFailed, true, "pg", "old"},
		{"backup_stale fires on old success", KindDatabaseBackupStale, true, "pg", "old"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, notice, err := EvaluateDatabaseRuleAll(context.Background(), live, src, Rule{Kind: tc.kind}, now)
			if err != nil {
				t.Fatal(err)
			}
			if out.Firing != tc.wantFire {
				t.Fatalf("firing=%v notice=%q, want %v", out.Firing, notice, tc.wantFire)
			}
			if tc.contains != "" && !strings.Contains(notice, tc.contains) {
				t.Fatalf("notice %q missing %q", notice, tc.contains)
			}
			if tc.excludes != "" && strings.Contains(notice, tc.excludes) {
				t.Fatalf("notice %q should not contain %q", notice, tc.excludes)
			}
		})
	}

	src.history["redis"] = []store.BackupHistory{{Status: store.BackupStatusSucceeded, FinishedAt: ts(50 * time.Hour)}}
	out, notice, err := EvaluateDatabaseRuleAll(context.Background(), live, src, Rule{Kind: KindDatabaseBackupStale}, now)
	if err != nil || !out.Firing || !strings.Contains(notice, "redis") {
		t.Fatalf("stale: firing=%v notice=%q err=%v", out.Firing, notice, err)
	}
}

func TestDeployFailedTrackerAndRule(t *testing.T) {
	now := time.Now()
	tr := NewDeployOutcomeTracker()
	r := Rule{Kind: KindDeployFailed}

	out, _, err := EvaluateDeployFailed(context.Background(), tr, r, now)
	if err != nil || out.Firing {
		t.Fatalf("empty tracker should not fire: %v %v", out.Firing, err)
	}

	tr.Record(DeployOutcome{AppName: "web", Image: "web:2", Error: "boom"}, now.Add(-5*time.Minute))
	out, notice, _ := EvaluateDeployFailed(context.Background(), tr, r, now)
	if !out.Firing || !strings.Contains(notice, "web -> web:2: boom") {
		t.Fatalf("failure should fire: %v %q", out.Firing, notice)
	}

	tr.Record(DeployOutcome{AppName: "web", Image: "web:3", Succeeded: true}, now)
	out, _, _ = EvaluateDeployFailed(context.Background(), tr, out, now)
	if out.Firing {
		t.Fatal("a later success must clear the failure")
	}

	tr.Record(DeployOutcome{AppName: "api", Image: "api:1"}, now.Add(-3*time.Hour))
	out, _, _ = EvaluateDeployFailed(context.Background(), tr, Rule{Kind: KindDeployFailed}, now)
	if out.Firing {
		t.Fatal("a failure older than the window must not fire")
	}
}

func TestDeployDispatcherFeedsTracker(t *testing.T) {
	db := newTestDB(t)
	tr := NewDeployOutcomeTracker()
	d := NewDeployDispatcher(db, nil, nil, nil, nil).WithOutcomeTracker(tr)
	d.Dispatch(context.Background(), "service:web", DeployOutcome{AppName: "web", Image: "web:9", Error: "x"})
	if got := tr.RecentDeployFailures(time.Now().Add(-time.Minute)); len(got) != 1 || got[0].AppName != "web" {
		t.Fatalf("dispatcher did not feed tracker: %+v", got)
	}
	if d.Outcomes() == nil {
		t.Fatal("Outcomes should expose the tracker")
	}
}

func TestDefaultSpecsIncludeCoverageKinds(t *testing.T) {
	have := map[Kind]bool{}
	for _, s := range DefaultRuleSpecs(DefaultsConfig{}) {
		have[s.Rule.Kind] = true
	}
	for _, k := range []Kind{KindBackupFailed, KindDeployFailed, KindDatabaseDown, KindDatabaseBackupStale} {
		if !have[k] {
			t.Errorf("default rule for %s missing", k)
		}
	}
}

func TestRouteDefaultRules(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	if err := db.SeedDefaults(ctx, DefaultsConfig{}); err != nil {
		t.Fatal(err)
	}

	n, err := db.RouteDefaultRules(ctx, "")
	if err != nil || n != 0 {
		t.Fatalf("no admin email must stay in-app: n=%d err=%v", n, err)
	}
	rules, _ := db.ListDefaultRules(ctx)
	for _, r := range rules {
		if r.ChannelID != "" {
			t.Fatalf("rule %s routed without email", r.ID)
		}
	}

	n, err = db.RouteDefaultRules(ctx, "admin@example.com")
	if err != nil || n != len(rules) {
		t.Fatalf("routed %d of %d: %v", n, len(rules), err)
	}
	rules, _ = db.ListDefaultRules(ctx)
	for _, r := range rules {
		if r.ChannelID != DefaultAdminEmailChannelID || r.NotifyKind != NotifyEmail || r.NotifyURL != "admin@example.com" {
			t.Fatalf("rule %s not routed to admin email: %+v", r.ID, r)
		}
	}

	// An operator detaching the channel is respected on the next boot.
	r := rules[0]
	r.ChannelID, r.NotifyURL, r.NotifyKind = "", "", ""
	if err := db.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	if n, err = db.RouteDefaultRules(ctx, "admin@example.com"); err != nil || n != 0 {
		t.Fatalf("detached rule re-routed: n=%d err=%v", n, err)
	}
}
