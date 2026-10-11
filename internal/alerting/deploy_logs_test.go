package alerting

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeBuildLogs struct{ lines []string }

func (f fakeBuildLogs) QueryDeployLog(_ context.Context, _ string) ([]telemetry.DeployLogEntry, error) {
	out := make([]telemetry.DeployLogEntry, len(f.lines))
	for i, l := range f.lines {
		out[i] = telemetry.DeployLogEntry{Message: l}
	}
	return out, nil
}

type fakeRuntimeLogs struct{ lines []string }

func (f fakeRuntimeLogs) QueryLogs(_ context.Context, _ string, _, _ time.Time, _ string) ([]telemetry.LogEntry, error) {
	out := make([]telemetry.LogEntry, len(f.lines))
	for i, l := range f.lines {
		out[i] = telemetry.LogEntry{Message: l}
	}
	return out, nil
}

func TestDeployDispatcherAttachLogs(t *testing.T) {
	many := make([]string, 300)
	for i := range many {
		many[i] = "line"
	}
	cases := []struct {
		name      string
		ev        DeployOutcome
		build     DeployLogSource
		runtime   LogsSource
		wantLines int
		wantFirst string
	}{
		{"succeeded gets nothing", DeployOutcome{Succeeded: true, AttemptID: "a"}, fakeBuildLogs{[]string{"x"}}, nil, 0, ""},
		{"build log preferred", DeployOutcome{AttemptID: "a"}, fakeBuildLogs{[]string{"npm ERR"}}, fakeRuntimeLogs{[]string{"rt"}}, 1, "npm ERR"},
		{"runtime fallback", DeployOutcome{AttemptID: "a"}, fakeBuildLogs{}, fakeRuntimeLogs{[]string{"rt"}}, 1, "rt"},
		{"capped at 200", DeployOutcome{AttemptID: "a"}, fakeBuildLogs{many}, nil, 200, "line"},
		{"no sources", DeployOutcome{AttemptID: "a"}, nil, nil, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &DeployDispatcher{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), runtime: tc.runtime, buildLogs: tc.build}
			ev := tc.ev
			d.attachLogs(context.Background(), "service:web", &ev)
			if len(ev.LogLines) != tc.wantLines {
				t.Fatalf("lines = %d, want %d", len(ev.LogLines), tc.wantLines)
			}
			if tc.wantLines > 0 && ev.LogLines[0] != tc.wantFirst {
				t.Errorf("first = %q", ev.LogLines[0])
			}
		})
	}
}

func TestSummaryDeployTextIncludesExcerpt(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "row " + strings.Repeat("x", 5)
	}
	lines[29] = "FATAL: cannot bind port"
	got := summaryDeployText(DeployOutcome{AppName: "web", Image: "img:1", Error: "build failed", LogLines: lines})
	if !strings.Contains(got, "FATAL: cannot bind port") || strings.Count(got, "row xxxxx") > deployLogExcerptLines {
		t.Errorf("text = %q", got)
	}
}
