package telemetry

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TestCollector_Live_RealContainerToRealStore proves the whole pipeline
// end to end against a real Docker daemon and a real (temp-file)
// SQLite database, not just that each piece works in isolation: a real
// container's real resource usage lands as real, queryable rows.
// Skips cleanly (not a failure) if Docker isn't reachable, the same
// pattern every other live test in this codebase uses.
func TestCollector_Live_RealContainerToRealStore(t *testing.T) {
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	// docker.Client exposes no public health-check method; InspectByName
	// against a name that shouldn't exist is a cheap, real round trip to
	// the daemon (a genuinely unreachable daemon errors here, a merely
	// absent container returns (nil, nil)), the lightest available way
	// from outside internal/docker to tell "no daemon" from "no such
	// container" before committing to the rest of this test.
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, pingErr := client.InspectByName(pingCtx, "levelrail-telemetry-live-test-connectivity-probe")
	cancelPing()
	if pingErr != nil {
		t.Skipf("docker daemon not reachable: %v", pingErr)
	}
	t.Cleanup(func() { _ = client.Close() })

	name := "levelrail-test-telemetry-collector"
	removeContainerIfExists(t, client, name)
	t.Cleanup(func() { removeContainerIfExists(t, client, name) })

	id, err := client.Create(context.Background(), docker.ContainerSpec{Name: name, Image: "nginx:alpine"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := client.Start(context.Background(), id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "telemetry.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	collector := NewCollector(client, store, time.Second, nil)
	if err := collector.CollectOnce(context.Background(), []Target{
		{ResourceID: "service:live-test", ContainerID: id},
	}); err != nil {
		t.Fatalf("CollectOnce() error = %v", err)
	}

	got, err := store.Query(context.Background(), "service:live-test", "memory_usage_bytes",
		time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Query() = %d samples, want exactly 1 written by CollectOnce", len(got))
	}
	if got[0].Value <= 0 {
		t.Errorf("memory_usage_bytes = %v, want a real positive figure for a running container", got[0].Value)
	}

	// Independent verification: every metric CollectOnce is documented
	// to write should be present, not just the one checked above.
	for _, metric := range []string{"cpu_percent", "memory_usage_bytes", "memory_limit_bytes", "network_rx_bytes", "network_tx_bytes", "disk_read_bytes", "disk_write_bytes"} {
		rows, err := store.Query(context.Background(), "service:live-test", metric, time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
		if err != nil {
			t.Fatalf("Query(%s) error = %v", metric, err)
		}
		if len(rows) != 1 {
			t.Errorf("Query(%s) = %d rows, want exactly 1", metric, len(rows))
		}
	}
}

func removeContainerIfExists(t *testing.T, c *docker.Client, name string) {
	t.Helper()
	state, err := c.InspectByName(context.Background(), name)
	if err != nil || state == nil {
		return
	}
	if err := c.Remove(context.Background(), state.ID, true); err != nil {
		t.Logf("cleanup: remove %s: %v", name, err)
	}
}
