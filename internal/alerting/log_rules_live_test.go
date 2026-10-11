package alerting

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dockersdk "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/strslice"
	dockerclient "github.com/docker/docker/client"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockertest"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// TestLogRules_Live_RealContainerSignaturesAndAlert drives the whole path
// against a real Docker daemon: a container writes two shapes of error line,
// the collector stores them, signatures cluster them into two groups with the
// right counts, and a log_pattern rule fires with redacted sample lines.
func TestLogRules_Live_RealContainerSignaturesAndAlert(t *testing.T) {
	dockertest.SkipIfShort(t)
	client, err := docker.NewClient()
	if err != nil {
		t.Skipf("no docker client available: %v", err)
	}
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	_, pingErr := client.InspectByName(pingCtx, "levelrail-log-rules-live-probe")
	cancelPing()
	if pingErr != nil {
		t.Skipf("docker daemon not reachable: %v", pingErr)
	}
	t.Cleanup(func() { _ = client.Close() })

	raw, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("new raw docker client: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	ctx := context.Background()
	rd, err := raw.ImagePull(ctx, "busybox:latest", image.PullOptions{})
	if err != nil {
		t.Fatalf("ImagePull: %v", err)
	}
	_, _ = io.Copy(io.Discard, rd)
	_ = rd.Close()

	script := `for i in 1 2 3; do echo "ERROR upstream 10.0.0.$i timed out after ${i}00ms token=s3cr3t"; done; ` +
		`for i in 7 8; do echo "ERROR cannot parse order 55$i"; done; echo "INFO warmed up"; sleep 15`
	name := "levelrail-test-log-rules-live"
	_ = raw.ContainerRemove(ctx, name, dockersdk.RemoveOptions{Force: true})
	resp, err := raw.ContainerCreate(ctx, &dockersdk.Config{Image: "busybox:latest", Cmd: strslice.StrSlice{"sh", "-c", script}}, nil, nil, nil, name)
	if err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}
	t.Cleanup(func() { _ = raw.ContainerRemove(context.Background(), resp.ID, dockersdk.RemoveOptions{Force: true}) })
	if err := client.Start(ctx, resp.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	store, err := telemetry.Open(ctx, filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatalf("telemetry.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	collector := telemetry.NewLogCollector(client, store, nil, nil)
	streamCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := collector.StreamOne(streamCtx, telemetry.LogTarget{ResourceID: "service:live-rules", ContainerID: resp.ID}); err != nil && streamCtx.Err() == nil {
		t.Fatalf("StreamOne: %v", err)
	}

	now := time.Now()
	entries, _, err := store.QueryLogsLimited(ctx, "service:live-rules", now.Add(-time.Minute), now.Add(time.Minute), "", 100)
	if err != nil {
		t.Fatalf("QueryLogsLimited: %v", err)
	}
	sigs := telemetry.AggregateSignatures(entries, now.Add(-time.Minute), now.Add(time.Minute), telemetry.SignatureOptions{MinLevel: telemetry.LevelError})
	if len(sigs) != 2 || sigs[0].Count != 3 || sigs[1].Count != 2 {
		t.Fatalf("signatures = %+v, want two clusters with counts 3 and 2", sigs)
	}

	rule := Rule{ID: "live", Kind: KindLogPattern, ResourceID: "service:live-rules", Enabled: true,
		LogRule: &LogRuleConfig{Query: "timed out", Count: 3, WindowSeconds: 300}}
	next, res, err := EvaluateLogRule(ctx, store, rule, LogRulePolicyFromEnv(), now)
	if err != nil {
		t.Fatalf("EvaluateLogRule: %v", err)
	}
	if !next.Firing || res.Matched != 3 || len(res.Samples) == 0 {
		t.Fatalf("firing=%v matched=%d samples=%v", next.Firing, res.Matched, res.Samples)
	}
	for _, s := range res.Samples {
		if strings.Contains(s, "s3cr3t") {
			t.Fatalf("sample leaks the secret: %q", s)
		}
	}
}
