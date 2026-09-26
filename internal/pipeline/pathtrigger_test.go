package pipeline

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func pathsYAML(on string) string {
	return "version: 1\non:\n" + on + "jobs:\n  t:\n    image: a\n    steps:\n      - run: echo hi\n"
}

func TestPathFilterSkipReasons(t *testing.T) {
	errFetch := errors.New("provider unavailable")
	tests := []struct {
		name       string
		on         string
		ev         Event
		wantRun    bool
		wantReason string
	}{
		{"push no path matched", "  push:\n    paths: [src/**]\n", Event{Kind: TriggerPush, Branch: "main", Changed: []string{"README.md"}}, false, "skipped: no changed path matched paths"},
		{"push path matched", "  push:\n    paths: [src/**]\n", Event{Kind: TriggerPush, Branch: "main", Changed: []string{"README.md", "src/a.go"}}, true, ""},
		{"push all ignored", "  push:\n    paths_ignore: ['**/*.md']\n", Event{Kind: TriggerPush, Branch: "main", Changed: []string{"a.md", "docs/b.md"}}, false, "skipped: every changed path matched paths_ignore"},
		{"push dotfile matches", "  push:\n    paths: ['.github/**']\n", Event{Kind: TriggerPush, Branch: "main", Changed: []string{".github/workflows/ci.yml"}}, true, ""},
		{"windows separators", "  push:\n    paths: ['src/**']\n", Event{Kind: TriggerPush, Branch: "main", Changed: []string{`src\a.go`}}, true, ""},
		{"lazy fetch supplies files", "  push:\n    paths: [src/**]\n", Event{Kind: TriggerPush, Branch: "main", ChangedFn: func(context.Context) ([]string, error) { return []string{"docs/x.md"}, nil }}, false, "skipped: no changed path matched paths"},
		{"fetch failure runs", "  push:\n    paths: [src/**]\n", Event{Kind: TriggerPush, Branch: "main", ChangedFn: func(context.Context) ([]string, error) { return nil, errFetch }}, true, ""},
		{"unknown files run", "  push:\n    paths: [src/**]\n", Event{Kind: TriggerPush, Branch: "main"}, true, ""},
		{"pr paths skip", "  pull_request:\n    paths: [api/**]\n", Event{Kind: TriggerPullRequest, Branch: "main", Action: "opened", Changed: []string{"web/a.ts"}}, false, "skipped: no changed path matched paths"},
		{"pr types skip", "  pull_request:\n    types: [synchronize]\n", Event{Kind: TriggerPullRequest, Branch: "main", Action: "opened"}, false, `action "opened" is not in on.pull_request.types`},
		{"pr reopened accepts opened", "  pull_request:\n    types: [reopened]\n", Event{Kind: TriggerPullRequest, Branch: "main", Action: "opened"}, true, ""},
		{"merge group runs", "  merge_group:\n    branches: [main]\n", Event{Kind: TriggerMergeGroup, Branch: "main"}, true, ""},
		{"merge group wrong branch", "  merge_group:\n    branches: [release]\n", Event{Kind: TriggerMergeGroup, Branch: "main"}, false, `does not match on.merge_group.branches`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newTriggerHarness(t)
			h.save(pathsYAML(tt.on))
			runs := h.trigger(tt.ev)
			if got := len(runs) == 1; got != tt.wantRun {
				t.Fatalf("started %d runs, want run=%v", len(runs), tt.wantRun)
			}
			log := h.triggerLog()
			if len(log) != 1 {
				t.Fatalf("trigger log = %+v", log)
			}
			if tt.wantRun {
				if log[0].Decision != store.TriggerStarted {
					t.Fatalf("decision = %s", log[0].Decision)
				}
				return
			}
			if log[0].Decision != store.TriggerSkipped || !strings.Contains(log[0].Reason, tt.wantReason) {
				t.Fatalf("log = %+v, want skipped with %q", log[0], tt.wantReason)
			}
		})
	}
}

func TestLazyChangedFilesFetchedOnce(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save(pathsYAML("  push:\n    paths: [src/**]\n"))
	h.save(strings.Replace(pathsYAML("  push:\n    paths_ignore: [docs/**]\n"), "version: 1", "version: 1\nname: other", 1))
	calls := 0
	ev := Event{Kind: TriggerPush, Branch: "main", ChangedFn: func(context.Context) ([]string, error) { calls++; return []string{"src/a.go"}, nil }}
	h.trigger(ev)
	if calls != 1 {
		t.Fatalf("changed files fetched %d times, want 1", calls)
	}
}

func TestNoFilterNeverFetchesChangedFiles(t *testing.T) {
	h, _ := newTriggerHarness(t)
	h.save(pathsYAML("  push:\n"))
	ev := Event{Kind: TriggerPush, Branch: "feat", ChangedFn: func(context.Context) ([]string, error) {
		t.Error("changed files fetched for a pipeline without path filters")
		return nil, nil
	}}
	if runs := h.trigger(ev); len(runs) != 1 {
		t.Fatalf("started %d runs, want 1", len(runs))
	}
}

type fakeReporter struct {
	mu    sync.Mutex
	calls []ReportRequest
	err   error
}

func (f *fakeReporter) ReportRun(_ context.Context, req ReportRequest) (ReportReceipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	return ReportReceipt{Provider: "github", URL: "https://forge.test/o/r/commit/" + req.SHA}, f.err
}

func (f *fakeReporter) states() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c.State)
	}
	return out
}

func runWithReporter(t *testing.T, rep StatusReporter, yamlText string) store.PipelineRun {
	t.Helper()
	h, _ := newTriggerHarness(t)
	h.e.cfg.Reporter = rep
	p := h.save(yamlText)
	run := h.start(p, StartOptions{Trigger: TriggerManual, SHA: "abc123", Ref: "refs/heads/main"})
	return h.done(run.ID)
}

func TestRunStateReportedToForge(t *testing.T) {
	rep := &fakeReporter{}
	run := runWithReporter(t, rep, pathsYAML("  manual: {}\n"))
	if run.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s %s", run.Status, run.Reason)
	}
	if got := strings.Join(rep.states(), ","); got != "pending,success" {
		t.Fatalf("states = %s, want pending,success", got)
	}
	if run.ReportProvider != "github" || run.ReportState != ReportSuccess || run.ReportURL == "" || run.ReportWarning != "" {
		t.Fatalf("report fields = %+v", run)
	}
	if ctxName := rep.calls[0].Context; ctxName != "t/pipeline/ci" {
		t.Fatalf("context = %q", ctxName)
	}
}

func TestFailedRunReportsFailure(t *testing.T) {
	rep := &fakeReporter{}
	y := "version: 1\non: { manual: {} }\njobs:\n  t:\n    image: a\n    steps:\n      - run: exit 3\n"
	h := newHarness(t, okHandler)
	h.e.cfg.Reporter = rep
	started := h.start(h.save(y), StartOptions{Trigger: TriggerManual, SHA: "abc123"})
	run := h.done(started.ID)
	if run.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", run.Status)
	}
	if got := strings.Join(rep.states(), ","); got != "pending,failure" {
		t.Fatalf("states = %s", got)
	}
}

func TestReportFailureNeverFailsRun(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"server error", errors.New("github api returned 500: boom"), "status not reported: github api returned 500"},
		{"rate limited", errors.New("rate limited by the git provider (retry after 60s)"), "rate limited by the git provider (retry after 60s)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := &fakeReporter{err: tt.err}
			run := runWithReporter(t, rep, pathsYAML("  manual: {}\n"))
			if run.Status != store.PipelineStatusSucceeded {
				t.Fatalf("a reporting failure changed the run to %s (%s)", run.Status, run.Reason)
			}
			if !strings.Contains(run.ReportWarning, tt.want) {
				t.Fatalf("warning = %q, want it to contain %q", run.ReportWarning, tt.want)
			}
			if run.ReportState != "" {
				t.Fatalf("state %q recorded for a failed post", run.ReportState)
			}
		})
	}
}

func TestReportSkippedWhenNoTargetOrDisabled(t *testing.T) {
	t.Run("no target", func(t *testing.T) {
		rep := &fakeReporter{err: ErrNoReportTarget}
		run := runWithReporter(t, rep, pathsYAML("  manual: {}\n"))
		if run.ReportProvider != "" || run.ReportWarning != "" {
			t.Fatalf("report fields set without a forge: %+v", run)
		}
	})
	t.Run("report_status false", func(t *testing.T) {
		rep := &fakeReporter{}
		y := strings.Replace(pathsYAML("  manual: {}\n"), "version: 1", "version: 1\nreport_status: false", 1)
		run := runWithReporter(t, rep, y)
		if len(rep.calls) != 0 || run.ReportProvider != "" {
			t.Fatalf("reported %d times with report_status false", len(rep.calls))
		}
	})
	t.Run("no commit", func(t *testing.T) {
		rep := &fakeReporter{}
		h, _ := newTriggerHarness(t)
		h.e.cfg.Reporter = rep
		run := h.start(h.save(pathsYAML("  manual: {}\n")), StartOptions{Trigger: TriggerManual})
		h.done(run.ID)
		if len(rep.calls) != 0 {
			t.Fatalf("reported %d times for a run with no commit", len(rep.calls))
		}
	})
}
