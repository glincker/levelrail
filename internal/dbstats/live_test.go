package dbstats

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
)

// dockerExec runs the collector script through docker exec, like the real exec path.
type dockerExec struct{}

func (dockerExec) ExecWithInput(ctx context.Context, id string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	c := exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", id}, cmd...)...) //nolint:gosec // opt in test, fixed binary
	c.Stdin = stdin
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	if err := c.Run(); err != nil {
		return nil, err
	}
	return io.NopCloser(&out), nil
}

type memWriter struct {
	mu sync.Mutex
	s  []telemetry.Sample
}

func (m *memWriter) WriteSamples(_ context.Context, s []telemetry.Sample) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s = append(m.s, s...)
	return nil
}

func (m *memWriter) take() map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]float64{}
	for _, s := range m.s {
		out[s.Metric] = s.Value
	}
	m.s = nil
	return out
}

func dockerRun(t *testing.T, args ...string) {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput() //nolint:gosec // opt in test, fixed binary
	if err != nil {
		t.Fatalf("docker %v: %v: %s", args[:2], err, out)
	}
}

const probe = `psql -U "$POSTGRES_USER" -d "$POSTGRES_USER" -Atc 'select 1'`

func startPostgres(t *testing.T, user string) string {
	t.Helper()
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "verify-dbstats-" + hex.EncodeToString(b)
	pw := hex.EncodeToString(b) + "pw"
	// The image only exports POSTGRES_USER when set, and the platform always sets it, so the test does too.
	args := []string{"run", "-d", "--name", name, "-e", "POSTGRES_PASSWORD=" + pw, "-e", "POSTGRES_USER=" + user, "-e", "POSTGRES_DB=" + user}
	dockerRun(t, append(args, "postgres:17")...)
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() }) //nolint:gosec // opt in test
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)
		// The image restarts once after init, so require two consecutive ready probes.
		if exec.Command("docker", "exec", name, "sh", "-c", probe).Run() == nil { //nolint:gosec // opt in test
			time.Sleep(2 * time.Second)
			if exec.Command("docker", "exec", name, "sh", "-c", probe).Run() == nil { //nolint:gosec // opt in test
				return name
			}
		}
	}
	t.Fatal("postgres not ready")
	return ""
}

func psqlIn(t *testing.T, name, sql string) {
	t.Helper()
	dockerRun(t, "exec", name, "sh", "-c", `psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_USER" -c "`+sql+`"`)
}

// TestLivePostgres is opt in: LEVELRAIL_DBSTATS_LIVE=1 with Docker running.
func TestLivePostgres(t *testing.T) {
	if os.Getenv("LEVELRAIL_DBSTATS_LIVE") != "1" {
		t.Skip("set LEVELRAIL_DBSTATS_LIVE=1 to run against a real Postgres container")
	}
	for _, admin := range []string{"postgres", "demo"} {
		name := startPostgres(t, admin)
		psqlIn(t, name, "CREATE DATABASE extra_one")
		psqlIn(t, name, "CREATE DATABASE extra_two")
		psqlIn(t, name, "CREATE TABLE t(id serial, v text); INSERT INTO t(v) SELECT repeat('x',100) FROM generate_series(1,5000)")

		w := &memWriter{}
		c := New(fakeSource{ts: []Target{{Name: "live", Engine: store.EnginePostgres, ContainerID: name}}}, dockerExec{}, w,
			func() Config { return Config{Timeout: 10 * time.Second} }, func() time.Duration { return time.Minute }, nil)
		ctx := context.Background()

		c.CollectOnce(ctx)
		if st := c.Status()["live"]; st.LastError != "" || st.LastOK.IsZero() {
			t.Fatalf("%s first tick: %+v", admin, st)
		}
		first := w.take()
		for _, m := range []string{telemetry.MetricDBConnActive, telemetry.MetricDBConnIdle, telemetry.MetricDBConnTotal, telemetry.MetricDBConnMax,
			telemetry.MetricDBLocksWaiting, telemetry.MetricDBSizePrefix + admin, telemetry.MetricDBSizePrefix + "extra_one", telemetry.MetricDBSizePrefix + "extra_two"} {
			if _, ok := first[m]; !ok {
				t.Errorf("%s: missing %s in %v", admin, m, first)
			}
		}
		if first[telemetry.MetricDBConnMax] != 100 || first[telemetry.MetricDBSizePrefix+admin] < 1<<20 {
			t.Errorf("%s: implausible values %v", admin, first)
		}
		// The collector's own session must be filtered out, so an idle server reports no active clients.
		if first[telemetry.MetricDBConnActive] != 0 {
			t.Errorf("%s: own backend counted as active: %v", admin, first[telemetry.MetricDBConnActive])
		}
		if _, ok := first[telemetry.MetricDBTupInserted]; ok {
			t.Errorf("first tick must not emit deltas")
		}

		psqlIn(t, name, "INSERT INTO t(v) SELECT 'y' FROM generate_series(1,1000)")
		psqlIn(t, name, "DROP DATABASE extra_two")
		time.Sleep(2 * time.Second) // stats flush
		c.CollectOnce(ctx)
		second := w.take()
		if got := second[telemetry.MetricDBTupInserted]; got < 1000 || got > 5000 {
			t.Errorf("%s: tup_inserted delta %v, want about 1000", admin, got)
		}
		if _, ok := second[telemetry.MetricDBSizePrefix+"extra_two"]; ok {
			t.Errorf("%s: dropped database still has a size series", admin)
		}
		if second[telemetry.MetricDBXactCommit] <= 0 {
			t.Errorf("%s: no commit delta: %v", admin, second)
		}
	}
}
