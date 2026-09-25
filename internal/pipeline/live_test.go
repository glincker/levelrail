package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestLiveJobsAgainstDocker runs a real two-job pipeline (artifact handoff, a
// step output, a failing step) against a local daemon and checks that no
// containers or workspace volumes are left behind.
func TestLiveJobsAgainstDocker(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	h := newHarness(t, nil)
	h.e.Close()
	h.e = New(Config{
		Store: h.db, NamePrefix: "pltest", Runtime: func(string) (Runtime, error) { return client, nil },
		Actions: h.act, MaxParallelJobs: 2,
	})
	t.Cleanup(h.e.Close)

	yaml := `
version: 1
jobs:
  make:
    image: alpine:3.20
    steps:
      - run: |
          mkdir -p out && echo built-by-live-test > out/result.txt
          echo "::set-output name=answer::42"
      - uses: artifact-upload
        with: { name: bin, path: out }
      - run: sh -c 'echo to-stderr >&2; exit 4'
        continue_on_error: true
  use:
    needs: [make]
    image: alpine:3.20
    steps:
      - uses: artifact-download
        with: { name: bin, path: got }
      - run: cat got/out/result.txt && echo answer=${{ needs.make.outputs.answer }}
`
	run := h.start(h.save(yaml), StartOptions{})
	got := h.until(run.ID, func(r store.PipelineRun) bool { return store.IsPipelineTerminal(r.Status) })
	if got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s (%s)\n%s\nlogs:\n%s", got.Status, got.Reason, h.dump(run.ID), h.logs(run.ID))
	}
	logs := h.logs(run.ID)
	for _, want := range []string{"built-by-live-test", "answer=42", "to-stderr"} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs missing %q:\n%s", want, logs)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	left, err := client.ListByPrefix(ctx, "pltest-pl-")
	if err != nil || len(left) != 0 {
		t.Errorf("containers left behind: %v %v", left, err)
	}
}
