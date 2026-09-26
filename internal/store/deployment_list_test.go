package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func seedDeployments(t *testing.T, db *DB, now time.Time) {
	t.Helper()
	ctx := context.Background()
	for _, n := range []string{"web", "api", "web-pr-7"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: n, Image: n + ":0", Port: 80}); err != nil {
			t.Fatalf("seed app: %v", err)
		}
	}
	atts := []DeployAttempt{
		{ID: "dep_1", ServiceName: "web", Image: "web:aaa", CommitSHA: "aaa111", Source: DeployAttemptSourceWebhook, Status: DeployAttemptStatusSucceeded, StartedAt: now.Add(-6 * time.Hour), CommitMessage: "add login", Branch: "main"},
		{ID: "dep_2", ServiceName: "web", Image: "web:bbb", CommitSHA: "bbb222", Source: DeployAttemptSourceWebhook, Status: DeployAttemptStatusSucceeded, StartedAt: now.Add(-5 * time.Hour), CommitMessage: "break things"},
		{ID: "dep_3", ServiceName: "web", Image: "web:aaa", Source: DeployAttemptSourceImage, Status: DeployAttemptStatusSucceeded, StartedAt: now.Add(-4 * time.Hour)},
		{ID: "dep_4", ServiceName: "api", Image: "api:ccc", CommitSHA: "ccc333", Source: DeployAttemptSourceManual, Status: DeployAttemptStatusFailed, StartedAt: now.Add(-3 * time.Hour)},
		{ID: "dep_5", ServiceName: "api", Image: "api:ddd", Source: DeployAttemptSourceManual, Status: DeployAttemptStatusRunning, StartedAt: now.Add(-time.Hour)},
		{ID: "dep_6", ServiceName: "web-pr-7", Image: "web-pr-7:eee", Source: DeployAttemptSourceWebhook, Status: DeployAttemptStatusHeld, StartedAt: now.Add(-30 * time.Minute)},
	}
	if err := db.SavePreviewEnvironment(ctx, PreviewEnvironment{ID: "pv_1", AppName: "web", PRNumber: 7, PreviewAppID: "web-pr-7", Branch: "feat", HeadSHA: "eee", Status: "ready", CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}); err != nil {
		t.Fatalf("seed preview: %v", err)
	}
	for _, a := range atts {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed attempt %s: %v", a.ID, err)
		}
	}
}

func ids(ds []Deployment) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Attempt.ID
	}
	return out
}

func TestListDeploymentsFilters(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name   string
		filter DeploymentFilter
		want   []string
	}{
		{"all newest first", DeploymentFilter{}, []string{"dep_6", "dep_5", "dep_4", "dep_3", "dep_2", "dep_1"}},
		{"status failed", DeploymentFilter{Statuses: []string{DeploymentFailed}}, []string{"dep_4"}},
		{"status building", DeploymentFilter{Statuses: []string{DeploymentBuilding}}, []string{"dep_5"}},
		{"status rolled back", DeploymentFilter{Statuses: []string{DeploymentRolledBack}}, []string{"dep_2"}},
		{"status ready", DeploymentFilter{Statuses: []string{DeploymentReady}}, []string{"dep_3", "dep_1"}},
		{"status held", DeploymentFilter{Statuses: []string{DeploymentHeld}}, []string{"dep_6"}},
		{"live only", DeploymentFilter{Live: true}, []string{"dep_3"}},
		{"pr", DeploymentFilter{PR: 7}, []string{"dep_6"}},
		{"trigger preview", DeploymentFilter{Triggers: []string{DeploymentTriggerPreview}}, []string{"dep_6"}},
		{"unknown status matches nothing", DeploymentFilter{Statuses: []string{"bogus"}}, nil},
		{"app", DeploymentFilter{App: "api"}, []string{"dep_5", "dep_4"}},
		{"trigger git push", DeploymentFilter{Triggers: []string{DeploymentTriggerGitPush}}, []string{"dep_2", "dep_1"}},
		{"trigger rollback", DeploymentFilter{Triggers: []string{DeploymentTriggerRollback}}, []string{"dep_3"}},
		{"trigger manual", DeploymentFilter{Triggers: []string{DeploymentTriggerManual}}, []string{"dep_5", "dep_4"}},
		{"query message", DeploymentFilter{Query: "login"}, []string{"dep_1"}},
		{"query sha prefix", DeploymentFilter{Query: "ccc"}, []string{"dep_4"}},
		{"query sha is prefix only", DeploymentFilter{Query: "c333"}, nil},
		{"query wildcard is literal", DeploymentFilter{Query: "%"}, nil},
		{"branch from webhook", DeploymentFilter{Branch: "main"}, []string{"dep_1"}},
		{"since", DeploymentFilter{Since: now.Add(-3*time.Hour - time.Minute)}, []string{"dep_6", "dep_5", "dep_4"}},
		{"until", DeploymentFilter{Until: now.Add(-5*time.Hour + time.Minute)}, []string{"dep_2", "dep_1"}},
		{"visible apps", DeploymentFilter{VisibleApps: []string{"api"}}, []string{"dep_5", "dep_4"}},
		{"visible apps empty", DeploymentFilter{VisibleApps: []string{}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			seedDeployments(t, db, now)
			got, _, err := db.ListDeployments(context.Background(), tt.filter)
			if err != nil {
				t.Fatalf("ListDeployments() error = %v", err)
			}
			g := ids(got)
			if len(g) != len(tt.want) {
				t.Fatalf("got %v, want %v", g, tt.want)
			}
			for i := range g {
				if g[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", g, tt.want)
				}
			}
		})
	}
}

func TestListDeploymentsDerivedFields(t *testing.T) {
	now := time.Now().UTC()
	db := openTestDB(t)
	seedDeployments(t, db, now)
	got, _, err := db.ListDeployments(context.Background(), DeploymentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Deployment{}
	for _, d := range got {
		byID[d.Attempt.ID] = d
	}
	if d := byID["dep_3"]; d.Trigger != DeploymentTriggerRollback || d.RollbackOf != "dep_1" {
		t.Errorf("dep_3 trigger=%q rollback_of=%q, want rollback of dep_1", d.Trigger, d.RollbackOf)
	}
	if d := byID["dep_2"]; d.Status != DeploymentRolledBack || d.Trigger != DeploymentTriggerGitPush || d.RolledBackBy != "dep_3" || d.IsLive {
		t.Errorf("dep_2 status=%q trigger=%q rolled_back_by=%q live=%v", d.Status, d.Trigger, d.RolledBackBy, d.IsLive)
	}
	if d := byID["dep_3"]; !d.IsLive {
		t.Error("dep_3 should be live")
	}
	if d := byID["dep_6"]; d.Trigger != DeploymentTriggerPreview || d.PRNumber != 7 || d.Status != DeploymentHeld {
		t.Errorf("dep_6 trigger=%q pr=%d status=%q", d.Trigger, d.PRNumber, d.Status)
	}
}

func TestListDeploymentsCursor(t *testing.T) {
	now := time.Now().UTC()
	db := openTestDB(t)
	seedDeployments(t, db, now)
	ctx := context.Background()

	var all []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		got, next, err := db.ListDeployments(ctx, DeploymentFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, ids(got)...)
		if next == "" {
			break
		}
		cursor = next
	}
	want := []string{"dep_6", "dep_5", "dep_4", "dep_3", "dep_2", "dep_1"}
	if len(all) != len(want) {
		t.Fatalf("paged = %v, want %v", all, want)
	}
	for i := range want {
		if all[i] != want[i] {
			t.Fatalf("paged = %v, want %v", all, want)
		}
	}

	if _, _, err := db.ListDeployments(ctx, DeploymentFilter{Cursor: "not-a-cursor"}); !errors.Is(err, ErrInvalidDeploymentCursor) {
		t.Errorf("bad cursor error = %v, want ErrInvalidDeploymentCursor", err)
	}
}

func TestBuildDeploymentSummary(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	fin := func(start time.Time, d time.Duration) *time.Time { f := start.Add(d); return &f }
	mk := func(status string, ago, dur time.Duration) DeploymentBrief {
		s := now.Add(-ago)
		b := DeploymentBrief{Status: status, StartedAt: s}
		if dur > 0 {
			b.FinishedAt = fin(s, dur)
		}
		return b
	}
	briefs := []DeploymentBrief{
		mk(DeploymentReady, time.Hour, 10*time.Second),
		mk(DeploymentReady, 2*time.Hour, 20*time.Second),
		mk(DeploymentReady, 3*time.Hour, 30*time.Second),
		mk(DeploymentFailed, 4*time.Hour, 40*time.Second),
		mk(DeploymentBuilding, 10*time.Minute, 0),
		mk(DeploymentReady, 30*time.Hour, 50*time.Second),
		mk(DeploymentFailed, 20*24*time.Hour, 5*time.Second),
	}
	tests := []struct {
		name       string
		window     time.Duration
		wantReady  int
		wantMedian int64
		wantP95    int64
	}{
		{"24h window", 24 * time.Hour, 3, 20000, 40000},
		{"7d window", 7 * 24 * time.Hour, 4, 30000, 50000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := BuildDeploymentSummary(briefs, now, tt.window)
			if s.Counts[DeploymentReady] != tt.wantReady {
				t.Errorf("ready = %d, want %d", s.Counts[DeploymentReady], tt.wantReady)
			}
			if s.MedianMS == nil || *s.MedianMS != tt.wantMedian {
				t.Errorf("median = %v, want %d", s.MedianMS, tt.wantMedian)
			}
			if s.P95MS == nil || *s.P95MS != tt.wantP95 {
				t.Errorf("p95 = %v, want %d", s.P95MS, tt.wantP95)
			}
			if s.InProgress != 1 {
				t.Errorf("in progress = %d, want 1", s.InProgress)
			}
			if s.FailureRate24h == nil || *s.FailureRate24h != 0.25 {
				t.Errorf("failure rate = %v, want 0.25", s.FailureRate24h)
			}
			if len(s.PerDay) != DeploymentSummaryDays || s.PerDay[DeploymentSummaryDays-1].Date != "2026-09-26" {
				t.Errorf("per day = %+v", s.PerDay)
			}
		})
	}

	empty := BuildDeploymentSummary(nil, now, 24*time.Hour)
	if empty.FailureRate24h != nil || empty.MedianMS != nil {
		t.Errorf("empty summary should have nil rate and median: %+v", empty)
	}
}
