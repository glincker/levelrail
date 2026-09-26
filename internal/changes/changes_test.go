package changes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeEvents struct {
	list []store.AppEvent
	err  error
}

func (f fakeEvents) ListAppEvents(context.Context, string, *store.AppEventCursor, time.Time, []string, int) ([]store.AppEvent, error) {
	return f.list, f.err
}

type fakeDeploys struct{ list []store.DeployAttempt }

func (f fakeDeploys) ListDeployAttempts(context.Context, string) ([]store.DeployAttempt, error) {
	return f.list, nil
}

type fakeAudit struct{ list []store.AuditEntry }

func (f fakeAudit) ListAuditEntries(_ context.Context, _ int, _ *time.Time, flt store.AuditEntryFilter) ([]store.AuditEntry, error) {
	var out []store.AuditEntry
	for _, e := range f.list {
		if strings.Contains(e.Path, flt.Search) {
			out = append(out, e)
		}
	}
	return out, nil
}

var now = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func ago(m int) time.Time { return now.Add(-time.Duration(m) * time.Minute) }

func agg(ev EventSource, dp DeploySource, au AuditSource) *Aggregator {
	return &Aggregator{Events: ev, Deploys: dp, Audit: au, Window: 30 * time.Minute, Max: 20}
}

func TestCollectOrderingAndSuspect(t *testing.T) {
	tests := []struct {
		name        string
		events      []store.AppEvent
		attempts    []store.DeployAttempt
		audit       []store.AuditEntry
		wantTitles  []string
		wantSuspect string
	}{
		{
			name:       "empty",
			wantTitles: []string{},
		},
		{
			name: "newest first across sources, nearest deploy is suspect",
			events: []store.AppEvent{
				{ID: "e1", Kind: store.AppEventEnvChange, Title: "Env changed: A", Keys: []string{"A"}, CreatedAt: ago(20), Actor: "bob"},
			},
			attempts: []store.DeployAttempt{
				{ID: "d1", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(5), Source: store.DeployAttemptSourceWebhook, ImageDigest: "sha256:abcdef0123456789"},
			},
			wantTitles:  []string{"Deploy to app:v2", "Env changed: A"},
			wantSuspect: "Deploy to app:v2",
		},
		{
			name: "failed deploy is listed but not suspected",
			events: []store.AppEvent{
				{ID: "e1", Kind: store.AppEventConfigChange, Title: "Config changed", CreatedAt: ago(20)},
			},
			attempts: []store.DeployAttempt{
				{ID: "d1", Image: "app:v3", Status: store.DeployAttemptStatusFailed, StartedAt: ago(2)},
			},
			wantTitles:  []string{"Deploy to app:v3 failed", "Config changed"},
			wantSuspect: "Config changed",
		},
		{
			name: "restart and freeze never suspected",
			events: []store.AppEvent{
				{ID: "e1", Kind: store.AppEventRestart, Title: "Restarted", CreatedAt: ago(1)},
				{ID: "e2", Kind: store.AppEventFreezeOverride, Title: "Deploy freeze overridden", CreatedAt: ago(2)},
				{ID: "e3", Kind: store.AppEventScale, Title: "Scaled to 3", CreatedAt: ago(9)},
			},
			wantTitles:  []string{"Restarted", "Deploy freeze overridden", "Scaled to 3"},
			wantSuspect: "Scaled to 3",
		},
		{
			name: "events outside the window and after until are dropped",
			events: []store.AppEvent{
				{ID: "e1", Kind: store.AppEventEnvChange, Title: "old", CreatedAt: ago(45)},
				{ID: "e2", Kind: store.AppEventEnvChange, Title: "future", CreatedAt: now.Add(time.Minute)},
			},
			wantTitles: []string{},
		},
		{
			name: "audit lb and maintenance entries",
			audit: []store.AuditEntry{
				{ID: "a1", Method: "PUT", Path: "/api/v1/apps/web/loadbalancer", StatusCode: 200, CreatedAt: store.FormatAuditTime(ago(4)), ActorName: "amy"},
				{ID: "a2", Method: "PUT", Path: "/api/v1/apps/web/domains/x.com/maintenance", StatusCode: 200, CreatedAt: store.FormatAuditTime(ago(6))},
				{ID: "a3", Method: "PUT", Path: "/api/v1/apps/web/loadbalancer", StatusCode: 403, CreatedAt: store.FormatAuditTime(ago(3))},
				{ID: "a4", Method: "GET", Path: "/api/v1/apps/web/loadbalancer", StatusCode: 200, CreatedAt: store.FormatAuditTime(ago(3))},
				{ID: "a5", Method: "PUT", Path: "/api/v1/apps/web2/loadbalancer", StatusCode: 200, CreatedAt: store.FormatAuditTime(ago(3))},
			},
			wantTitles:  []string{"Load balancer updated", "Domain maintenance updated"},
			wantSuspect: "Load balancer updated",
		},
		{
			name: "rollback detected and suspected",
			attempts: []store.DeployAttempt{
				{ID: "d3", Image: "app:v1", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(3)},
				{ID: "d2", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(60)},
				{ID: "d1", Image: "app:v1", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(120)},
			},
			wantTitles:  []string{"Rollback to app:v1"},
			wantSuspect: "Rollback to app:v1",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := agg(fakeEvents{list: tc.events}, fakeDeploys{list: tc.attempts}, fakeAudit{list: tc.audit}).Collect(context.Background(), "web", now)
			got := make([]string, len(res.Changes))
			for i, c := range res.Changes {
				got[i] = c.Title
			}
			if strings.Join(got, "|") != strings.Join(tc.wantTitles, "|") {
				t.Fatalf("titles = %v, want %v", got, tc.wantTitles)
			}
			s := res.Suspect()
			switch {
			case tc.wantSuspect == "" && s != nil:
				t.Fatalf("unexpected suspect %q", s.Title)
			case tc.wantSuspect != "" && (s == nil || s.Title != tc.wantSuspect):
				t.Fatalf("suspect = %+v, want %q", s, tc.wantSuspect)
			}
			flagged := 0
			for _, c := range res.Changes {
				if c.LikelyCause {
					flagged++
				}
			}
			if flagged > 1 {
				t.Fatalf("%d changes flagged, want at most 1", flagged)
			}
		})
	}
}

func TestCollectDeployActorAndDigest(t *testing.T) {
	res := agg(nil, fakeDeploys{list: []store.DeployAttempt{
		{ID: "d1", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(5), Source: store.DeployAttemptSourceWebhook, Author: "carol", ImageDigest: "sha256:abcdef0123456789ff", CommitSHA: "1234567890"},
	}}, nil).Collect(context.Background(), "web", now)
	c := res.Changes[0]
	if c.Actor != "carol" || !strings.Contains(c.Detail, "digest abcdef012345") || !strings.Contains(c.Detail, "commit 1234567") {
		t.Fatalf("unexpected change %+v", c)
	}
}

func TestCollectCapAndTruncation(t *testing.T) {
	var evs []store.AppEvent
	for i := 0; i < 8; i++ {
		evs = append(evs, store.AppEvent{ID: "e" + string(rune('a'+i)), Kind: store.AppEventEnvChange, Title: "t", CreatedAt: ago(i + 1)})
	}
	a := agg(fakeEvents{list: evs}, nil, nil)
	a.Max = 3
	res := a.Collect(context.Background(), "web", now)
	if len(res.Changes) != 3 || res.Total != 8 || !res.Truncated {
		t.Fatalf("got len=%d total=%d truncated=%v", len(res.Changes), res.Total, res.Truncated)
	}
	if !res.Changes[0].LikelyCause {
		t.Fatal("newest env change should be the suspect")
	}
}

func TestCollectSourceErrorIsSkipped(t *testing.T) {
	res := agg(fakeEvents{err: errors.New("boom")}, fakeDeploys{list: []store.DeployAttempt{
		{ID: "d1", Image: "app:v2", Status: store.DeployAttemptStatusSucceeded, StartedAt: ago(5)},
	}}, nil).Collect(context.Background(), "web", now)
	if len(res.Changes) != 1 {
		t.Fatalf("want the deploy despite the event source failing, got %d", len(res.Changes))
	}
}

func TestEnvFallbacks(t *testing.T) {
	t.Setenv(EnvWindow, "45m")
	t.Setenv(EnvMaxEntries, "7")
	if WindowFromEnv() != 45*time.Minute || MaxFromEnv() != 7 {
		t.Fatal("env not honoured")
	}
	t.Setenv(EnvWindow, "nonsense")
	t.Setenv(EnvMaxEntries, "-1")
	if WindowFromEnv() != DefaultWindow || MaxFromEnv() != DefaultMaxEntries {
		t.Fatal("bad env should fall back to defaults")
	}
}

func TestNotifyBlockCaps(t *testing.T) {
	long := strings.Repeat("x", 500)
	var cs []Change
	for i := 0; i < 9; i++ {
		cs = append(cs, Change{At: ago(i), Kind: KindEnv, Title: "Env changed", Detail: long, Actor: "bob"})
	}
	cs[0].LikelyCause = true
	res := Result{Window: 30 * time.Minute, Changes: cs, Total: 12}

	lines := NotifyLines(res)
	if len(lines) != NotifyMaxLines+1 {
		t.Fatalf("lines = %d, want %d", len(lines), NotifyMaxLines+1)
	}
	if lines[NotifyMaxLines] != "and 7 more" {
		t.Fatalf("last line = %q", lines[NotifyMaxLines])
	}
	for _, l := range lines[:NotifyMaxLines] {
		// 5 chars of "HH:MM " prefix plus the clipped line.
		if utf8.RuneCountInString(l) > NotifyLineCap+6 {
			t.Fatalf("line too long: %d", utf8.RuneCountInString(l))
		}
	}
	block := NotifyBlock(res, "https://x.example/apps/web/alerts")
	if !strings.HasPrefix(block, "Changed in the last 30m:") || !strings.HasSuffix(block, "https://x.example/apps/web/alerts") {
		t.Fatalf("unexpected block %q", block)
	}
	if len(block) > 1000 {
		t.Fatalf("block is %d bytes", len(block))
	}
	if NotifyBlock(Result{}, "l") != "" {
		t.Fatal("empty result should render nothing")
	}
}

func TestLineMarksLikelyCauseAndKeys(t *testing.T) {
	c := Change{Title: "Env changed", Keys: []string{"A", "B"}, Actor: "bob", LikelyCause: true}
	if got := c.Line(); got != "Env changed (A, B; by bob) [likely cause]" {
		t.Fatalf("line = %q", got)
	}
}
