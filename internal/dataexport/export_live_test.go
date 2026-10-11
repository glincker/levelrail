package dataexport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const lokiImage = "grafana/loki:3.3.2"

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func pull(ctx context.Context, t *testing.T, cli *dockerclient.Client, ref string) {
	t.Helper()
	r, err := cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		t.Skipf("cannot pull %s (offline?): %v", ref, err)
	}
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()
}

func removeContainer(cli *dockerclient.Client, id string) {
	_ = cli.ContainerRemove(context.Background(), id, container.RemoveOptions{Force: true})
}

// TestExport_Live_LogsReachLokiAndAuditReachesSink drives the real path: a
// real container's stdout goes through LogCollector and LogBroadcaster into
// the export service and out to a real Loki, while an audit row reaches a
// webhook sink, and a dead destination alongside them changes nothing.
func TestExport_Live_LogsReachLokiAndAuditReachesSink(t *testing.T) {
	dockertest.SkipIfShort(t)
	t.Setenv(AllowPrivateEnv, "true")
	dc, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client: %v", err)
	}
	t.Cleanup(func() { _ = dc.Close() })
	raw, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	ctx := context.Background()
	if _, err := raw.Ping(ctx); err != nil {
		t.Skipf("docker daemon not reachable: %v", err)
	}

	pull(ctx, t, raw, lokiImage)
	pull(ctx, t, raw, "busybox:latest")

	port := freePort(t)
	lokiPort := nat.Port("3100/tcp")
	loki, err := raw.ContainerCreate(ctx,
		&container.Config{Image: lokiImage, ExposedPorts: nat.PortSet{lokiPort: {}}},
		&container.HostConfig{PortBindings: nat.PortMap{lokiPort: {{HostIP: "127.0.0.1", HostPort: strconv.Itoa(port)}}}},
		nil, nil, "")
	if err != nil {
		t.Fatalf("create loki: %v", err)
	}
	t.Cleanup(func() { removeContainer(raw, loki.ID) })
	if err := raw.ContainerStart(ctx, loki.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start loki: %v", err)
	}
	lokiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitFor90 := time.Now().Add(120 * time.Second)
	for {
		resp, err := http.Get(lokiURL + "/ready") //nolint:gosec,noctx // local test endpoint
		if err == nil {
			ok := resp.StatusCode == http.StatusOK
			_ = resp.Body.Close()
			if ok {
				break
			}
		}
		if time.Now().After(waitFor90) {
			t.Fatal("loki did not become ready")
		}
		time.Sleep(time.Second)
	}

	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "cp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tdb, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "tel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tdb.Close() })

	broadcaster := telemetry.NewLogBroadcaster()
	cfg := fastConfig()
	cfg.BackoffBase = 100 * time.Millisecond
	svc := NewService(Deps{Store: db, Secrets: &memSecrets{}, Logs: broadcaster, Config: cfg, Brand: "plat"})

	audit := &sink{}
	auditSrv := httptest.NewServer(audit)
	defer auditSrv.Close()

	for _, in := range []Input{
		{Name: "loki", Kind: store.ExportKindLogsLoki, Endpoint: lokiURL, Enabled: true, Apps: []string{"live-export"}},
		{Name: "audit", Kind: store.ExportKindAuditWebhook, Endpoint: auditSrv.URL, Enabled: true, HMACKey: "0123456789abcdef"},
		{Name: "dead", Kind: store.ExportKindLogsHTTP, Endpoint: fmt.Sprintf("http://127.0.0.1:%d", freePort(t)), Enabled: true},
	} {
		if _, err := svc.Create(ctx, in); err != nil {
			t.Fatalf("create %s: %v", in.Name, err)
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go svc.Run(runCtx)
	waitFor(t, "workers", func() bool { svc.mu.Lock(); defer svc.mu.Unlock(); return len(svc.workers) == 3 })

	app, err := raw.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd: strslice.StrSlice{"sh", "-c",
			`i=0; while [ $i -lt 20 ]; do echo '{"level":"warn","msg":"live-export-line"}'; i=$((i+1)); sleep 1; done`},
	}, nil, nil, nil, "")
	if err != nil {
		t.Fatalf("create app container: %v", err)
	}
	t.Cleanup(func() { removeContainer(raw, app.ID) })
	if err := raw.ContainerStart(ctx, app.ID, container.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	collector := telemetry.NewLogCollector(dc, tdb, broadcaster, nil)
	streamCtx, stopStream := context.WithTimeout(ctx, 25*time.Second)
	defer stopStream()
	go func() {
		_ = collector.StreamOne(streamCtx, telemetry.LogTarget{ResourceID: "service:live-export", ContainerID: app.ID})
	}()

	if err := db.SaveAuditEntry(ctx, store.AuditEntry{
		ID: "aud_live", ActorType: "token", ActorName: "ci", Ability: "write", Method: "POST",
		Path: "/api/v1/apps/live-export/deploy", StatusCode: 202, CreatedAt: store.FormatAuditTime(time.Now().UTC().Add(time.Second)), ClientKind: "api",
	}); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "audit row at sink", func() bool {
		for _, l := range audit.lines() {
			if strings.Contains(l, `"id":"aud_live"`) {
				return true
			}
		}
		return false
	})

	deadline := time.Now().Add(60 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		q := url.Values{
			"query": {`{app="live-export"}`},
			"start": {strconv.FormatInt(time.Now().Add(-10*time.Minute).UnixNano(), 10)},
			"end":   {strconv.FormatInt(time.Now().Add(time.Minute).UnixNano(), 10)},
		}
		resp, err := http.Get(lokiURL + "/loki/api/v1/query_range?" + q.Encode()) //nolint:gosec,noctx // local test endpoint
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var doc struct {
				Data struct {
					Result []struct {
						Stream map[string]string `json:"stream"`
						Values [][2]string       `json:"values"`
					} `json:"result"`
				} `json:"data"`
			}
			if json.Unmarshal(b, &doc) == nil && len(doc.Data.Result) > 0 && len(doc.Data.Result[0].Values) > 0 {
				got = doc.Data.Result[0].Values[0][1]
				if doc.Data.Result[0].Stream["level"] != "warn" || doc.Data.Result[0].Stream["app"] != "live-export" {
					t.Fatalf("loki labels = %v", doc.Data.Result[0].Stream)
				}
				break
			}
		}
		time.Sleep(time.Second)
	}
	if !strings.Contains(got, "live-export-line") {
		t.Fatalf("log line never reached loki, last value %q", got)
	}

	id := svc.mustID(t, "dead")
	waitFor(t, "dead destination failures counted", func() bool {
		st, _ := svc.Status(ctx, id)
		return st.FailuresTotal > 0 && st.LastError != ""
	})
}
