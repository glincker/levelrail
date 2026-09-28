package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeSource struct {
	url, token string
	err        error
}

func (f fakeSource) RepoInfo(context.Context, string) (string, string, error) {
	return f.url, f.token, f.err
}

func TestJobRetryRerunsWholeJob(t *testing.T) {
	t.Parallel()
	calls := 0
	h := newHarness(t, func(s string) execResult {
		if strings.Contains(s, "first-step") {
			calls++
		}
		if strings.Contains(s, "flaky") && calls < 2 {
			return execResult{Exit: 1}
		}
		return execResult{Output: "ok\n"}
	})
	yaml := "version: 1\njobs:\n  t:\n    image: a\n    retries: 1\n    steps:\n      - run: first-step\n      - run: flaky\n"
	run := h.start(h.save(yaml), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s (%s)\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	if calls != 2 {
		t.Errorf("first-step ran %d times, want 2 (whole job re-run)", calls)
	}
	if !strings.Contains(h.logs(run.ID), "retrying") {
		t.Error("retry not logged")
	}
}

func TestCheckoutUsesTokenWithoutLeakingIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	h.e.cfg.Source = fakeSource{url: "https://example.test/org/app.git", token: "tok-abcdef-123456"}
	run := h.start(h.save("version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo hi\n"), StartOptions{SHA: "abc123", Ref: "refs/heads/main"})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s (%s)\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	if h.rt.ran("git remote add origin 'https://example.test/org/app.git'") != 1 || h.rt.ran("checkout -q --force 'abc123'") != 1 {
		t.Errorf("checkout script missing: %v", h.rt.scripts)
	}
	if h.rt.ran("http.extraHeader") != 1 {
		t.Error("token was not sent as an auth header")
	}
	for _, s := range h.rt.scripts {
		if strings.Contains(s, "tok-abcdef-123456") && !strings.Contains(s, "http.extraHeader") {
			t.Errorf("raw token outside the auth header: %q", s)
		}
	}
	if strings.Contains(h.logs(run.ID), "tok-abcdef-123456") {
		t.Error("token leaked into logs")
	}
	if h.rt.liveContainers() != 0 {
		t.Errorf("checkout container leaked: %d", h.rt.liveContainers())
	}
}

func TestCheckoutSkippedWithoutRepository(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	h.e.cfg.Source = fakeSource{err: ErrNoRepo}
	run := h.start(h.save("version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo hi\n"), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s (%s)", got.Status, got.Reason)
	}
	if h.rt.ran("git remote add") != 0 {
		t.Error("checkout ran for an app with no repository")
	}
}

func TestCancelAfterRestartCleansOrphanedJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t, func(string) execResult { return execResult{Block: true} })
	run := h.start(h.save("version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: sleep 100\n"), StartOptions{})
	h.until(run.ID, func(store.PipelineRun) bool { return h.rt.ran("sleep 100") > 0 })
	h.e.Close()
	if h.rt.liveContainers() == 0 {
		t.Fatal("expected an orphaned container after the interrupt")
	}

	h.e = h.newEngine()
	if err := h.e.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusCancelled {
		t.Fatalf("run = %s (%s)", got.Status, got.Reason)
	}
	if h.rt.liveContainers() != 0 {
		t.Errorf("orphaned container not removed: %d", h.rt.liveContainers())
	}
	for _, s := range h.jobs(run.ID)["t"].Steps {
		if s.Status == store.PipelineStatusRunning {
			t.Errorf("step %d left running after cancel", s.Index)
		}
	}
}

func TestJobRunsAfterFailedNeedWhenConditionAllows(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := `
version: 1
jobs:
  build:
    image: a
    steps:
      - run: exit 3
  cleanup:
    needs: [build]
    if: needs.build.result == 'failure'
    image: a
    steps:
      - run: echo cleaning
  ship:
    needs: [build]
    image: a
    steps:
      - run: echo shipping
`
	run := h.start(h.save(yaml), StartOptions{})
	got := h.done(run.ID)
	if got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	jobs := h.jobs(run.ID)
	if jobs["cleanup"].Status != store.PipelineStatusSucceeded || jobs["ship"].Status != store.PipelineStatusSkipped {
		t.Fatalf("cleanup=%s ship=%s\n%s", jobs["cleanup"].Status, jobs["ship"].Status, h.dump(run.ID))
	}
	if h.rt.ran("echo shipping") != 0 {
		t.Error("ship ran despite a failed dependency")
	}
}

func TestRunLoopCompletesAndStops(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	run := h.start(h.save("version: 1\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo hi\n"), StartOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.e.Run(ctx, 10*time.Millisecond) }()

	deadline := time.After(10 * time.Second)
	for {
		r, err := h.db.GetPipelineRun(context.Background(), run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if store.IsPipelineTerminal(r.Status) {
			if r.Status != store.PipelineStatusSucceeded {
				t.Fatalf("run = %s (%s)", r.Status, r.Reason)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("run loop never finished the run: %s", r.Status)
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	if err := <-done; err != context.Canceled {
		t.Errorf("Run returned %v, want context.Canceled", err)
	}
}

func TestParseMemory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want int64
		bad  bool
	}{
		{"", 0, false}, {"512", 512, false}, {"2Mi", 2 << 20, false}, {"1Gi", 1 << 30, false}, {"3K", 3000, false}, {"x", 0, true},
	}
	for _, tc := range tests {
		got, err := parseMemory(tc.in)
		if (err != nil) != tc.bad || got != tc.want {
			t.Errorf("parseMemory(%q) = %d, %v", tc.in, got, err)
		}
	}
}

func TestTargetAppsListsOnlyOtherApps(t *testing.T) {
	t.Parallel()
	def, issues := Validate([]byte(`
version: 1
jobs:
  a:
    steps:
      - uses: deploy
        with: { service: web, image: x }
      - uses: promote
        with: { from: staging, to: web }
      - uses: notify
        with: { message: m, app: pager }
`))
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	got := TargetApps(def, "web")
	if strings.Join(got, ",") != "pager,staging" {
		t.Errorf("TargetApps = %v", got)
	}
	if _, issues := Validate([]byte("version: 1\njobs:\n  a:\n    steps:\n      - uses: deploy\n        with: { service: '${{ inputs.x }}', image: y }\n")); len(issues) == 0 {
		t.Error("an expression as a deploy target must be rejected")
	}
}
