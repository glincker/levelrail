package alerting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeChangeSource struct {
	res   changes.Result
	calls []string
}

func (f *fakeChangeSource) Collect(_ context.Context, app string, _ time.Time) changes.Result {
	f.calls = append(f.calls, app)
	return f.res
}

func sampleChanges(n int) changes.Result {
	res := changes.Result{App: "web", Window: 30 * time.Minute, WindowSec: 1800, Total: n}
	for i := 0; i < n; i++ {
		res.Changes = append(res.Changes, changes.Change{
			At: time.Date(2026, 9, 26, 11, 59-i, 0, 0, time.UTC), Kind: changes.KindEnv,
			Title: "Env changed", Keys: []string{"DATABASE_URL"}, Actor: "bob", LikelyCause: i == 0,
		})
	}
	return res
}

func TestSummaryTextIncludesChangesForFiringOnly(t *testing.T) {
	res := sampleChanges(8)
	ev := Event{Rule: Rule{Name: "5xx", Kind: KindThreshold, ResourceID: "service:web"}, Changes: &res, ChangesLink: "https://d.example/apps/web/alerts"}

	got := summaryText(ev)
	if !strings.Contains(got, "Changed in the last 30m:") || !strings.Contains(got, "[likely cause]") {
		t.Fatalf("missing changes section:\n%s", got)
	}
	if !strings.Contains(got, "and 3 more") || !strings.HasSuffix(got, "https://d.example/apps/web/alerts") {
		t.Fatalf("missing overflow line or link:\n%s", got)
	}
	if strings.Count(got, "Env changed") != changes.NotifyMaxLines {
		t.Fatalf("want %d change lines:\n%s", changes.NotifyMaxLines, got)
	}
	if strings.Contains(got, "DATABASE_URL=") {
		t.Fatal("values must never appear")
	}

	ev.Resolved = true
	if strings.Contains(summaryText(ev), "Changed in the last") {
		t.Fatal("resolved events must not carry changes")
	}
}

func TestSummaryTextCappedKeepsChangesAndRespectsLimit(t *testing.T) {
	res := sampleChanges(8)
	logs := make([]string, 200)
	for i := range logs {
		logs[i] = strings.Repeat("l", 80)
	}
	ev := Event{Rule: Rule{Name: "crash", Kind: KindCrashloop, ResourceID: "service:web"}, LogLines: logs, Changes: &res, ChangesLink: "https://d.example/x"}

	for _, limit := range []int{discordMaxContent, pagerDutyMaxSummary} {
		got := summaryTextCapped(ev, limit)
		if len(got) > limit {
			t.Fatalf("len = %d, limit %d", len(got), limit)
		}
		if !strings.Contains(got, "Changed in the last 30m:") || !strings.HasSuffix(got, "https://d.example/x") {
			t.Fatalf("changes section lost at limit %d", limit)
		}
	}
	small := Event{Rule: Rule{Name: "x", Kind: KindThreshold}}
	if summaryTextCapped(small, 100) != summaryText(small) {
		t.Fatal("short messages must pass through unchanged")
	}
}

func TestNotifiersCarryChanges(t *testing.T) {
	res := sampleChanges(7)
	ev := Event{Rule: Rule{ID: "r", Name: "5xx", Kind: KindThreshold, ResourceID: "service:web"}, Changes: &res, ChangesLink: "https://d.example/l"}

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name   string
		notify notifyFunc
		check  func(t *testing.T)
	}{
		{"slack", notifySlack, func(t *testing.T) {
			var p slackPayload
			mustDecode(t, body, &p)
			if !strings.Contains(p.Text, "Changed in the last 30m") {
				t.Fatal(p.Text)
			}
		}},
		{"discord", notifyDiscord, func(t *testing.T) {
			var p discordPayload
			mustDecode(t, body, &p)
			if !strings.Contains(p.Content, "likely cause") || len(p.Content) > discordMaxContent {
				t.Fatal(p.Content)
			}
		}},
		{"generic", notifyGeneric, func(t *testing.T) {
			var p genericPayload
			mustDecode(t, body, &p)
			if len(p.RecentChanges) != changes.NotifyMaxLines || !p.RecentChanges[0].LikelyCause || p.ChangesLink != "https://d.example/l" {
				t.Fatalf("%+v", p)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body = nil
			if err := tc.notify(context.Background(), srv.Client(), srv.URL, ev); err != nil {
				t.Fatal(err)
			}
			tc.check(t)
		})
	}
}

func mustDecode(t *testing.T, b []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode %q: %v", b, err)
	}
}

func TestEngineAttachesChangesToFiringAppAlerts(t *testing.T) {
	res := sampleChanges(2)
	src := &fakeChangeSource{res: res}
	metrics := &fakeMetricsSource{samples: nil}
	r := Rule{ID: "r1", Name: "cpu", Kind: KindThreshold, ResourceID: "service:web", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 1, Enabled: true}
	rules := newFakeRuleStore(r)
	metrics.samples = []telemetry.Sample{{Timestamp: time.Now(), Value: 50}}
	spy := &spyNotifier{}
	e := newTestEngine(rules, metrics, nil, nil, spy)
	e.SetChanges(src, func(app string) string { return "https://d.example/apps/" + app + "/alerts" })

	if err := e.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := spy.calls()
	if len(calls) != 1 || calls[0].Changes == nil || len(calls[0].Changes.Changes) != 2 || calls[0].ChangesLink != "https://d.example/apps/web/alerts" {
		t.Fatalf("calls = %+v", calls)
	}
	if len(src.calls) != 1 || src.calls[0] != "web" {
		t.Fatalf("collect calls = %v", src.calls)
	}

	platform := Rule{ID: "r2", Name: "certs", Kind: KindThreshold, ResourceID: "platform", Metric: "cpu_percent", Comparator: GreaterThan, Threshold: 1, Enabled: true}
	rules2 := newFakeRuleStore(platform)
	spy2 := &spyNotifier{}
	e2 := newTestEngine(rules2, metrics, nil, nil, spy2)
	src2 := &fakeChangeSource{res: res}
	e2.SetChanges(src2, nil)
	if err := e2.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(src2.calls) != 0 || spy2.calls()[0].Changes != nil {
		t.Fatal("non-app rules must not collect changes")
	}
}
