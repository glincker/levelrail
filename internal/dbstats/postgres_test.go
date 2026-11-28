package dbstats

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
)

const scalarFixture = "conn_active=2\nconn_idle=5\nconn_total=7\nconn_max=100\nlocks_waiting=1\nfuture=9\n" +
	"db=5|100|900|100|10|4|1|0|50|2\ndb=6|0|10|1|1|1|1|1|1|1\n"

const sizeFixture = "size.app=2048\nsize.postgres=1024\nsize.extra=512\n"

func TestParseScalars(t *testing.T) {
	res, err := parseScalars(scalarFixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Gauges) != 5 || len(res.Rows) != 2 || res.Rows[5].Counters[0] != 900 || res.Rows[5].Reset != 100 {
		t.Fatalf("bad parse: %+v", res)
	}
	for _, bad := range []string{"garbage", "conn_total=x", "conn_total=1\ndb=1|2", "db=x|1|1|1|1|1|1|1|1|1"} {
		if _, err := parseScalars(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestParseSizesCaps(t *testing.T) {
	vals, err := parseSizes(sizeFixture, 2)
	if err != nil || len(vals) != 2 || vals[0].Metric != telemetry.MetricDBSizePrefix+"app" {
		t.Fatalf("vals=%v err=%v", vals, err)
	}
}

func row(reset int64, c float64) dbRow {
	r := dbRow{Reset: reset}
	for i := range r.Counters {
		r.Counters[i] = c
	}
	return r
}

func TestCounterDeltas(t *testing.T) {
	sum := func(prev, cur map[int64]dbRow) float64 { return counterDeltas(prev, cur)[0].Value }
	cases := []struct {
		name      string
		prev, cur map[int64]dbRow
		want      float64
	}{
		{"growth", map[int64]dbRow{1: row(1, 10), 2: row(1, 5)}, map[int64]dbRow{1: row(1, 15), 2: row(1, 6)}, 6},
		{"drop database", map[int64]dbRow{1: row(1, 10), 2: row(1, 500)}, map[int64]dbRow{1: row(1, 12)}, 2},
		{"new database", map[int64]dbRow{1: row(1, 10)}, map[int64]dbRow{1: row(1, 11), 3: row(1, 900)}, 1},
		{"pg_stat_reset", map[int64]dbRow{1: row(1, 1000)}, map[int64]dbRow{1: row(2, 3)}, 0},
		{"restart counters back", map[int64]dbRow{1: row(0, 1000)}, map[int64]dbRow{1: row(0, 3)}, 0},
	}
	for _, tc := range cases {
		if got := sum(tc.prev, tc.cur); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

type fakeExec struct {
	mu     sync.Mutex
	calls  []string
	fail   map[string]bool
	scalar string
	size   string
}

func (f *fakeExec) ExecWithInput(_ context.Context, id string, cmd []string, _ io.Reader) (io.ReadCloser, error) {
	f.mu.Lock()
	f.calls = append(f.calls, id)
	f.mu.Unlock()
	if f.fail[id] {
		return nil, errors.New("container gone")
	}
	s := cmd[2]
	for _, want := range []string{"default_transaction_read_only=on", "statement_timeout=", "lock_timeout=1000", "PGCONNECT_TIMEOUT=5", "levelrail-stats"} {
		if !strings.Contains(s, want) {
			return nil, errors.New("script missing " + want)
		}
	}
	if strings.Contains(s, "pg_database_size") {
		return io.NopCloser(strings.NewReader(f.size)), nil
	}
	return io.NopCloser(strings.NewReader(f.scalar)), nil
}

type fakeSource struct{ ts []Target }

func (f fakeSource) RunningDatabases(context.Context) ([]Target, error) { return f.ts, nil }

type fakeWriter struct {
	mu  sync.Mutex
	got []telemetry.Sample
}

func (f *fakeWriter) WriteSamples(_ context.Context, s []telemetry.Sample) error {
	f.mu.Lock()
	f.got = append(f.got, s...)
	f.mu.Unlock()
	return nil
}

func newTest(ex *fakeExec, w *fakeWriter, ts ...Target) *Collector {
	return New(fakeSource{ts: ts}, ex, w, func() Config { return Config{} },
		func() time.Duration { return time.Second }, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func has(w *fakeWriter, metric string) bool {
	for _, s := range w.got {
		if s.Metric == metric {
			return true
		}
	}
	return false
}

func TestCollectOnceDeltasAndIsolation(t *testing.T) {
	ex := &fakeExec{fail: map[string]bool{"bad": true}, scalar: scalarFixture, size: sizeFixture}
	w := &fakeWriter{}
	c := newTest(ex, w,
		Target{Name: "main", Engine: store.EnginePostgres, ContainerID: "c1"},
		Target{Name: "broken", Engine: store.EnginePostgres, ContainerID: "bad"},
		Target{Name: "cache", Engine: store.EngineValkey, ContainerID: "c2"})
	c.CollectOnce(context.Background())
	if has(w, telemetry.MetricDBXactCommit) {
		t.Fatal("first sample must not write counter deltas")
	}
	if !has(w, telemetry.MetricDBConnMax) || !has(w, telemetry.MetricDBSizePrefix+"app") {
		t.Fatalf("gauges missing: %v", w.got)
	}
	for _, id := range ex.calls {
		if id == "c2" {
			t.Fatal("non postgres database was probed")
		}
	}
	ex.scalar = strings.Replace(scalarFixture, "db=5|100|900|100|10|4|1|0|50|2", "db=5|100|950|100|10|4|1|0|60|2", 1)
	c.CollectOnce(context.Background())
	if !has(w, telemetry.MetricDBXactCommit) || !has(w, telemetry.MetricDBBlksHit) {
		t.Fatal("second sample must write deltas")
	}
	if st := c.Status(); st["broken"].LastError == "" || st["main"].LastOK.IsZero() {
		t.Fatalf("status = %+v", st)
	}
}

func TestSizeFailureKeepsScalars(t *testing.T) {
	ex := &fakeExec{scalar: scalarFixture, size: "not a size line"}
	w := &fakeWriter{}
	newTest(ex, w, Target{Name: "main", Engine: store.EnginePostgres, ContainerID: "c1"}).CollectOnce(context.Background())
	if !has(w, telemetry.MetricDBConnTotal) {
		t.Fatal("scalar metrics lost when sizes failed")
	}
}

func TestOutputOverCapRejected(t *testing.T) {
	ex := &fakeExec{scalar: scalarFixture + strings.Repeat("x", 20000), size: sizeFixture}
	w := &fakeWriter{}
	newTest(ex, w, Target{Name: "main", Engine: store.EnginePostgres, ContainerID: "c1"}).CollectOnce(context.Background())
	if len(w.got) != 0 {
		t.Fatal("truncated output was stored")
	}
}

func TestConfigNormalize(t *testing.T) {
	c := Config{}.normalize()
	if c.Timeout < time.Second || c.MaxDatabases != 20 || c.MaxOutputBytes <= 0 {
		t.Fatalf("%+v", c)
	}
	if (Config{MaxDatabases: 9999}).normalize().MaxDatabases != 200 {
		t.Fatal("ceiling not applied")
	}
}

func TestScalarMetricsRegistered(t *testing.T) {
	for _, m := range scalarMetrics {
		if !telemetry.IsDBMetric(m) {
			t.Errorf("%q unregistered", m)
		}
	}
	for _, m := range counterMetrics {
		if telemetry.KindOf(m) != telemetry.KindDelta {
			t.Errorf("%q must be a delta", m)
		}
	}
}
