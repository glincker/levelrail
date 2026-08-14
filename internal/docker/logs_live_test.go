package docker

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/strslice"
)

// TestClient_Logs_Live proves Logs actually demultiplexes a real
// container's stdout and stderr into correctly-tagged, timestamped lines,
// reading from the Docker Engine API directly (the observability design
// rule that logs never come from the json-file driver's on-disk files).
//
// ContainerSpec has no command-override field (stats_test.go's live test
// comment notes the same gap), so this test builds the container directly
// through the raw SDK client rather than this package's own Create,
// purely to get deterministic, distinguishable stdout/stderr output to
// assert against.
func TestClient_Logs_Live(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	name := "levelrail-test-docker-logs"
	removeIfExists(ctx, t, c, name)
	t.Cleanup(func() { removeIfExists(context.Background(), t, c, name) })

	if err := c.ensureImage(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ensureImage() error = %v", err)
	}

	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image: "busybox:latest",
			Cmd:   strslice.StrSlice{"sh", "-c", "echo out-line-1; echo err-line-1 1>&2; sleep 15"},
		},
		nil, nil, nil, name,
	)
	if err != nil {
		t.Fatalf("ContainerCreate() error = %v", err)
	}
	if err := c.Start(ctx, resp.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	lines, errs := c.Logs(ctx, resp.ID, true, time.Time{})

	var gotStdout, gotStderr bool
	deadline := time.After(15 * time.Second)
	for !gotStdout || !gotStderr {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatalf("lines channel closed before observing both stdout and stderr, got stdout=%v stderr=%v", gotStdout, gotStderr)
			}
			if line.Timestamp.IsZero() {
				t.Errorf("line %+v has a zero Timestamp, want Docker's own recorded time (Logs always requests Timestamps: true)", line)
			}
			switch {
			case line.Stream == "stdout" && line.Message == "out-line-1":
				gotStdout = true
			case line.Stream == "stderr" && line.Message == "err-line-1":
				gotStderr = true
			}
		case err, ok := <-errs:
			if ok && err != nil {
				t.Fatalf("log stream error: %v", err)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for both stdout and stderr lines: stdout=%v stderr=%v", gotStdout, gotStderr)
		}
	}
}
