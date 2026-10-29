package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

var fixedEnd = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

type fakeTel struct {
	empty bool
	boom  bool
}

func (f fakeTel) QueryMetrics(_ context.Context, _, metric string, from, to time.Time) ([]telemetry.Sample, error) {
	if f.boom {
		panic("telemetry exploded")
	}
	if f.empty {
		return nil, nil
	}
	var out []telemetry.Sample
	i := 0
	for ts := from; ts.Before(to); ts = ts.Add(30 * time.Minute) {
		i++
		switch metric {
		case metricMemory:
			v := 100e6 + float64(i%3)*1e6
			if to.Sub(ts) < 90*time.Minute {
				v = 125e6
			}
			out = append(out, telemetry.Sample{Timestamp: ts, Value: v})
		case telemetry.MetricContainerRestartCount:
			if to.Sub(ts) < 2*time.Hour {
				out = append(out, telemetry.Sample{Timestamp: ts, Value: 1})
			}
		}
	}
	return out, nil
}

func (f fakeTel) QueryLogs(_ context.Context, _ string, from, _ time.Time, _ string) ([]telemetry.LogEntry, error) {
	if f.empty {
		return nil, nil
	}
	var out []telemetry.LogEntry
	for i := 0; i < 6; i++ {
		out = append(out, telemetry.LogEntry{Timestamp: from.Add(time.Duration(i) * time.Minute), Message: "ERROR db timeout after " + string(rune('1'+i)) + "00ms password=hunter2hunter2 Authorization: Bearer abcdefghijklmnop123456"})
	}
	return append(out, telemetry.LogEntry{Timestamp: from, Message: "listening on :8080"}), nil
}

type fakeDeploys struct{}

func (fakeDeploys) ListDeployAttempts(context.Context, string) ([]store.DeployAttempt, error) {
	return []store.DeployAttempt{{ID: "d1", StartedAt: fixedEnd.Add(-20 * time.Minute), Status: store.DeployAttemptStatusFailed, Error: "boom"}}, nil
}

type fakeServices struct{}

func (fakeServices) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return &store.DesiredService{Resources: &store.ServiceResources{MemoryBytes: 128e6}}, nil
}

func testCfg() Config {
	return ConfigFromEnv(func(k string) (string, bool) { return map[string]string{"APP_ANALYSIS_ENABLED": "on"}[k], true })
}

func TestDigestAndBuilderAreStableRedactedAndHonestWhenEmpty(t *testing.T) {
	cfg := testCfg()
	src := Sources{Telemetry: fakeTel{}, Deploys: fakeDeploys{}, Services: fakeServices{}}
	tr := Trigger{Kind: TriggerAlert, ID: "h1", App: "web", RuleName: "crashloop", At: fixedEnd}

	build := func() []byte {
		b, err := json.Marshal(Build(tr, Collect(context.Background(), src, cfg, "web", fixedEnd), cfg, fixedEnd))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	first := build()
	if string(first) != string(build()) {
		t.Fatal("report output is not deterministic")
	}
	for _, secret := range []string{"hunter2hunter2", "abcdefghijklmnop123456"} {
		if strings.Contains(string(first), secret) {
			t.Fatalf("report leaked %q", secret)
		}
	}
	var rep Report
	if err := json.Unmarshal(first, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Status != StatusComplete || len(rep.Causes) < 3 {
		t.Fatalf("status %q causes %d", rep.Status, len(rep.Causes))
	}
	if rep.Causes[0].ID != "recent_deploy" || rep.Causes[0].Evidence[0] != "deploy:d1" || rep.Causes[1].ID != "restarts" {
		t.Fatalf("unexpected ranking %+v", rep.Causes)
	}
	var errGroups int
	for _, g := range rep.Digest.Logs {
		if g.Level == "error" {
			errGroups++
			if g.Count != 6 {
				t.Fatalf("error group count %d, want 6 (numbers must be masked)", g.Count)
			}
		}
	}
	if errGroups != 1 {
		t.Fatalf("error groups %d, want 1", errGroups)
	}

	empty := Build(tr, Collect(context.Background(), Sources{Telemetry: fakeTel{empty: true}}, cfg, "web", fixedEnd), cfg, fixedEnd)
	if empty.Status != StatusInsufficient || !strings.HasPrefix(empty.Summary, "Insufficient data") || len(empty.Causes) != 0 {
		t.Fatalf("empty report = %+v", empty)
	}
}

type countingSaver struct {
	saves atomic.Int32
	err   error
}

func (s *countingSaver) SaveAnalysisReport(context.Context, store.AnalysisReport) error {
	s.saves.Add(1)
	return s.err
}
func (s *countingSaver) PruneAnalysisReports(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func TestTriggerIsOffByDefaultAndFailureIsolated(t *testing.T) {
	off := ConfigFromEnv(func(string) (string, bool) { return "", false })
	if off.Enabled || NewRunner(off, Sources{}, &countingSaver{}, nil) != nil {
		t.Fatal("analysis must be off unless APP_ANALYSIS_ENABLED=on")
	}
	var nilRunner *Runner
	nilRunner.OnAlert(alerting.HistoryEntry{Event: alerting.EventFired, App: "web"})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	fired := alerting.HistoryEntry{ID: "h1", Event: alerting.EventFired, App: "web", At: fixedEnd}

	saver := &countingSaver{err: errors.New("disk full")}
	r := NewRunner(testCfg(), Sources{Telemetry: fakeTel{}}, saver, logger)
	r.now = func() time.Time { return fixedEnd }
	r.OnAlert(fired)
	r.OnAlert(fired)
	r.Wait()
	if got := saver.saves.Load(); got != 1 {
		t.Fatalf("saves = %d, want 1 (second firing is rate limited, save error is swallowed)", got)
	}

	pr := NewRunner(testCfg(), Sources{Telemetry: fakeTel{boom: true}}, &countingSaver{}, logger)
	pr.now = func() time.Time { return fixedEnd }
	pr.OnAlert(fired)
	pr.Wait()
}
