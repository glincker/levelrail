package api

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dockersdk "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TestHandleLiveLogStream_Live_BlueGreenOverlap_HidesDeadContainerFromCurrentView
// reproduces the exact regression against a real Docker daemon and a
// real SQLite store, not fakes: an old and new container briefly
// coexist, the old one is killed once the new one is current, and its
// shutdown line lands with a LATER timestamp than the new container's
// own quiet last line. Before this fix the live view sorted by
// timestamp alone and showed the dead container's line as current.
// Skips cleanly if Docker isn't reachable (see logs_live_test.go).
func TestHandleLiveLogStream_Live_BlueGreenOverlap_HidesDeadContainerFromCurrentView(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, pingErr := client.InspectByName(pingCtx, "levelrail-api-bluegreen-logs-connectivity-probe")
	cancelPing()
	if pingErr != nil {
		t.Skipf("docker daemon not reachable: %v", pingErr)
	}
	t.Cleanup(func() { _ = client.Close() })

	ctx := context.Background()
	const appName = "levelrail-test-api-bluegreen-logs"
	const oldImage = "old-image:v1"
	const newImage = "new-image:v2"

	// The exact name formula the reconciler, exec, and the fix itself
	// (currentAppContainerIDs) all derive independently from
	// store.DesiredService: old and new get different names because they
	// differ by image, which is what lets both exist side by side during
	// a real cutover.
	oldName := application.ContainerName(appName, oldImage, "")
	newName := application.ContainerName(appName, newImage, "")

	removeAPILogTestContainerIfExists(t, client, oldName)
	removeAPILogTestContainerIfExists(t, client, newName)
	t.Cleanup(func() {
		removeAPILogTestContainerIfExists(t, client, oldName)
		removeAPILogTestContainerIfExists(t, client, newName)
	})

	rawCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("new raw docker client: %v", err)
	}
	t.Cleanup(func() { _ = rawCli.Close() })

	pullReader, err := rawCli.ImagePull(ctx, "busybox:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("ImagePull() error = %v", err)
	}
	if _, err := io.Copy(io.Discard, pullReader); err != nil {
		t.Fatalf("reading image pull progress stream: %v", err)
	}
	_ = pullReader.Close()

	// The new (current) container: one line, then quiet, exactly the
	// "healthy, just hasn't logged anything since" shape that made the
	// dead container's chronologically-later shutdown line look live.
	newID := createAPILogTestContainer(ctx, t, rawCli, client, newName,
		`echo new-container-ready; sleep 60`)
	// The old (dying) container: one startup line, then waits for
	// SIGTERM (what client.Stop below sends) to log its shutdown line and
	// exit, the same shape n8n's own SIGTERM handler produced in the real
	// incident this fix responds to.
	oldID := createAPILogTestContainer(ctx, t, rawCli, client, oldName,
		`echo old-container-still-serving; trap 'echo old-container-shutting-down; exit 0' TERM; sleep 60 & wait`)

	tdb := newTestTelemetryDB(t)
	broadcaster := telemetry.NewLogBroadcaster()
	collector := telemetry.NewLogCollector(client, tdb, broadcaster, nil)
	resourceID := "service:" + appName

	collectCtx, cancelCollect := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for _, target := range []telemetry.LogTarget{
		{ResourceID: resourceID, ContainerID: oldID},
		{ResourceID: resourceID, ContainerID: newID},
	} {
		wg.Add(1)
		go func(tg telemetry.LogTarget) {
			defer wg.Done()
			_ = collector.StreamOne(collectCtx, tg)
		}(target)
	}

	waitForLogMessage(t, tdb, resourceID, "new-container-ready", 10*time.Second)
	waitForLogMessage(t, tdb, resourceID, "old-container-still-serving", 10*time.Second)

	// Cutover completing: the old container is killed once the new one
	// is current. Its shutdown line is written with a LATER timestamp
	// than the new container's own last line above, reproducing the
	// exact race this fix targets.
	if err := client.Stop(ctx, oldID, 5*time.Second); err != nil {
		t.Fatalf("Stop(old) error = %v", err)
	}
	waitForLogMessage(t, tdb, resourceID, "old-container-shutting-down", 10*time.Second)

	cancelCollect()
	wg.Wait()

	// Sanity check: prove the race actually happened in real data, not
	// merely asserted. Without this, a timing fluke could make the rest
	// of this test pass for the wrong reason.
	all, err := tdb.QueryLogs(ctx, resourceID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "")
	if err != nil {
		t.Fatalf("QueryLogs() error = %v", err)
	}
	if len(all) == 0 {
		t.Fatal("QueryLogs() returned no entries, test setup failed to collect anything")
	}
	if last := all[len(all)-1]; last.Message != "old-container-shutting-down" {
		t.Fatalf("setup did not reproduce the race: last entry by ts = %q (container %q), want the dead container's shutdown line to sort last",
			last.Message, last.ContainerID)
	}

	// Now exercise the actual fix: a real Router, with svc.Image ==
	// newImage so currentAppContainerIDs (log_live_container.go) resolves
	// "current" to newID via the exact same application.ContainerName
	// formula used to create it above, wired to the same real Docker
	// client exec.go's resolveExecContainer would also use.
	db := openTestDB(t)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: appName, Image: newImage, Port: 8080}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	resolver := func(string) (docker.Runtime, error) { return client, nil }
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)),
		WithLogBroadcaster(broadcaster),
		WithExecRuntime(resolver),
	)

	srv := httptest.NewServer(rt.Handler())
	defer srv.Close()
	cookie := loginViaServer(t, srv, db)

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer reqCancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/api/v1/apps/"+appName+"/logs/stream", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	reader := bufio.NewReader(resp.Body)

	first, err := nextSSEData(reader)
	if err != nil {
		t.Fatalf("read backfill event: %v", err)
	}
	if !strings.Contains(first, "new-container-ready") {
		t.Fatalf("first live event = %q, want the current (new) container's own line, not the dead one's", first)
	}
	if strings.Contains(first, "old-container") {
		t.Fatalf("first live event = %q, must never contain the dead container's output", first)
	}

	// The real assertion: the old container's two lines (including its
	// later-timestamped shutdown message, the exact precondition proven
	// above) must never appear as "live," full stop. With the live
	// endpoint already drained of its one legitimate (current-container)
	// line, any further read either blocks until reqCtx's own 3s timeout
	// (success: nothing else was ever sent) or, if the fix regressed,
	// would return the dead container's line instead.
	second, err := nextSSEData(reader)
	if err == nil {
		t.Fatalf("received a second live event = %q, want none: the dead container's lines must never reach the live view", second)
	}
}

func createAPILogTestContainer(ctx context.Context, t *testing.T, rawCli *dockerclient.Client, inspector *docker.Client, name, shellCmd string) string {
	t.Helper()
	resp, err := rawCli.ContainerCreate(ctx,
		&dockersdk.Config{
			Image: "busybox:latest",
			Cmd:   strslice.StrSlice{"sh", "-c", shellCmd},
		},
		nil, nil, nil, name,
	)
	if err != nil {
		t.Fatalf("ContainerCreate(%s) error = %v", name, err)
	}
	if err := inspector.Start(ctx, resp.ID); err != nil {
		t.Fatalf("Start(%s) error = %v", name, err)
	}
	return resp.ID
}

// waitForLogMessage polls tdb until message has been written under
// resourceID or timeout elapses, so this test never depends on a fixed
// sleep to outguess real container startup and log-flush timing
// (logBatchMaxWait in internal/telemetry/logs.go).
func waitForLogMessage(t *testing.T, tdb *telemetry.DB, resourceID, message string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		entries, err := tdb.QueryLogs(context.Background(), resourceID,
			time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "")
		if err == nil {
			for _, e := range entries {
				if e.Message == message {
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for log message %q under resource %q", message, resourceID)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func removeAPILogTestContainerIfExists(t *testing.T, c *docker.Client, name string) {
	t.Helper()
	state, err := c.InspectByName(context.Background(), name)
	if err != nil || state == nil {
		return
	}
	if err := c.Remove(context.Background(), state.ID, true); err != nil {
		t.Logf("cleanup: remove %s: %v", name, err)
	}
}
