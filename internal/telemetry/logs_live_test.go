package telemetry

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	dockersdk "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
)

// TestLogCollector_Live_RealContainerToRealStore proves the whole log
// pipeline end to end against a real Docker daemon and a real (temp-file)
// SQLite database, the log-store counterpart to
// TestCollector_Live_RealContainerToRealStore: a real container's real
// stdout/stderr output lands as real, independently queryable rows,
// searchable by full text, not just that StreamOne returns without
// erroring. Skips cleanly (not a failure) if Docker isn't reachable, the
// same pattern every other live test in this codebase uses.
func TestLogCollector_Live_RealContainerToRealStore(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, pingErr := client.InspectByName(pingCtx, "levelrail-telemetry-logs-live-test-connectivity-probe")
	cancelPing()
	if pingErr != nil {
		t.Skipf("docker daemon not reachable: %v", pingErr)
	}
	t.Cleanup(func() { _ = client.Close() })

	name := "levelrail-test-telemetry-log-collector"
	removeLogTestContainerIfExists(t, client, name)
	t.Cleanup(func() { removeLogTestContainerIfExists(t, client, name) })

	ctx := context.Background()

	// ContainerSpec (internal/docker) has no command-override field, the
	// same gap internal/docker's own live tests already document, so this
	// builds the container through a second, independent raw SDK client
	// (the same "second client for capabilities the wrapper doesn't
	// expose" pattern cmd/levelrail's own BuildKit wiring already uses)
	// for deterministic, structured-and-plain output to assert against.
	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("new raw docker client: %v", err)
	}
	t.Cleanup(func() { _ = rawCli.Close() })

	// docker.Client's own ensureImage (unexported, internal/docker only)
	// isn't reachable from this package, so pull explicitly: busybox
	// isn't already guaranteed present the way nginx:alpine is via
	// internal/docker's own live tests.
	pullReader, err := rawCli.ImagePull(ctx, "busybox:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("ImagePull() error = %v", err)
	}
	if _, err := io.Copy(io.Discard, pullReader); err != nil {
		t.Fatalf("reading image pull progress stream: %v", err)
	}
	_ = pullReader.Close()

	resp, err := rawCli.ContainerCreate(ctx,
		&dockersdk.Config{
			Image: "busybox:latest",
			Cmd: strslice.StrSlice{"sh", "-c",
				`echo plain-text-line; echo '{"level":"info","msg":"structured-line"}'; sleep 15`},
		},
		nil, nil, nil, name,
	)
	if err != nil {
		t.Fatalf("ContainerCreate() error = %v", err)
	}
	if err := client.Start(ctx, resp.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "telemetry.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	collector := NewLogCollector(client, store, nil, nil)
	streamCtx, cancelStream := context.WithTimeout(ctx, 8*time.Second)
	defer cancelStream()

	err = collector.StreamOne(streamCtx, LogTarget{ResourceID: "service:live-log-test", ContainerID: resp.ID})
	if err != nil && streamCtx.Err() == nil {
		t.Fatalf("StreamOne() error = %v", err)
	}
	// A deadline-based stop (not the container exiting on its own) is the
	// expected shutdown path here; context.DeadlineExceeded from
	// StreamOne, or the ctx.Err() check above having already fired, are
	// both the same "we asked it to stop" outcome, not a failure.

	got, err := store.QueryLogs(context.Background(), "service:live-log-test",
		time.Now().Add(-time.Minute), time.Now().Add(time.Minute), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("QueryLogs() = %d entries, want exactly 2 (the two lines the container emitted): %+v", len(got), got)
	}

	var sawPlain, sawStructured bool
	for _, e := range got {
		if e.Message == "plain-text-line" {
			sawPlain = true
			if e.Structured {
				t.Errorf("plain-text-line classified Structured = true, want false")
			}
		}
		if e.Structured && e.FieldsJSON != "" {
			sawStructured = true
			if e.FieldsJSON != `{"level":"info","msg":"structured-line"}` {
				t.Errorf("FieldsJSON = %q, want the exact JSON the container emitted", e.FieldsJSON)
			}
		}
		if e.Stream != "stdout" {
			t.Errorf("entry %+v has Stream = %q, want stdout (both test lines went to stdout)", e, e.Stream)
		}
	}
	if !sawPlain {
		t.Error("did not find the plain-text line among the queried entries")
	}
	if !sawStructured {
		t.Error("did not find the structured (JSON) line among the queried entries")
	}

	// Independent verification: full-text search over the same data,
	// proving the FTS5 index (migrations/0002_log_entries.sql) is
	// actually wired up against real written rows, not just exercised
	// against the synthetic fixtures in logs_test.go.
	matched, err := store.QueryLogs(context.Background(), "service:live-log-test",
		time.Now().Add(-time.Minute), time.Now().Add(time.Minute), "structured-line")
	if err != nil {
		t.Fatalf("QueryLogs() with full-text query error = %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("QueryLogs(query=\"structured-line\") = %d entries, want exactly 1", len(matched))
	}
}

func removeLogTestContainerIfExists(t *testing.T, c *docker.Client, name string) {
	t.Helper()
	state, err := c.InspectByName(context.Background(), name)
	if err != nil || state == nil {
		return
	}
	if err := c.Remove(context.Background(), state.ID, true); err != nil {
		t.Logf("cleanup: remove %s: %v", name, err)
	}
}
