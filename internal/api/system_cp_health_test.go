package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCPDB struct{ checkpoints, optimizes, resets int }

func (f *fakeCPDB) Metrics() store.DBMetrics {
	return store.DBMetrics{WALBytes: 200 << 20, P95MS: 1, ReadPoolEnabled: true, ReadPoolSize: 4, ReadWaitCount: 9}
}
func (f *fakeCPDB) CheckpointWAL(context.Context, bool) error {
	f.checkpoints++
	return nil
}
func (f *fakeCPDB) Optimize(context.Context) error { f.optimizes++; return nil }
func (f *fakeCPDB) ResetSlowQueries()              { f.resets++ }

func TestCPHealthFixAndFindings(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	fake := &fakeCPDB{}
	rt.SetControlPlaneProbe(ControlPlaneProbe{DB: fake, Reconcile: func() reconcile.Stats { return reconcile.Stats{} }})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/control-plane-health", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"wal_size"`) {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"read_pool_enabled":true`, `"read_pool_size":4`, `"read_wait_count":9`, `"wait_count":0`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response lacks %s: %s", want, rec.Body.String())
		}
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/control-plane-health/fix", `{"fix":"wal_checkpoint"}`))
	if rec.Code != http.StatusOK || fake.checkpoints != 1 {
		t.Fatalf("fix = %d, checkpoints %d, body %s", rec.Code, fake.checkpoints, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/control-plane-health/fix", `{"fix":"rm -rf"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown fix = %d", rec.Code)
	}
}

func TestCPHealthReconcileQueue(t *testing.T) {
	cases := []struct {
		name  string
		stats reconcile.Stats
		want  bool
	}{
		{"pool", reconcile.Stats{Queue: &reconcile.QueueStats{Workers: 4, Depth: 3, InFlight: 2, OldestWait: 1500 * time.Millisecond}}, true},
		{"single worker", reconcile.Stats{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			rt.SetControlPlaneProbe(ControlPlaneProbe{DB: &fakeCPDB{}, Reconcile: func() reconcile.Stats { return tc.stats }})
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/control-plane-health", ""))
			var body struct {
				Reconcile map[string]json.RawMessage `json:"reconcile"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			raw, ok := body.Reconcile["queue"]
			if ok != tc.want {
				t.Fatalf("queue present = %v, want %v: %s", ok, tc.want, rec.Body.String())
			}
			if !tc.want {
				return
			}
			var q cpQueue
			if err := json.Unmarshal(raw, &q); err != nil {
				t.Fatal(err)
			}
			if q != (cpQueue{Workers: 4, Depth: 3, InFlight: 2, OldestWaitSeconds: 1.5}) {
				t.Fatalf("queue = %+v", q)
			}
		})
	}
}

func TestLiveStreamCap(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.cpHealth.cap = 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	if _, ok := rt.startLiveSSE(httptest.NewRecorder(), req); !ok {
		t.Fatal("first stream refused")
	}
	rec := httptest.NewRecorder()
	if _, ok := rt.startLiveSSE(rec, req); ok || rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("second stream = ok %v code %d, want refused with 503", ok, rec.Code)
	}
}

type fakeTelemetry struct{ compacts int }

func (f *fakeTelemetry) FileBytes(context.Context) (int64, error) { return 900 << 20, nil }
func (f *fakeTelemetry) Compact(context.Context) error            { f.compacts++; return nil }

func TestCPHealthTelemetryFindingAndFix(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	tel := &fakeTelemetry{}
	rt.SetControlPlaneProbe(ControlPlaneProbe{DB: &fakeCPDB{}, Telemetry: tel, Reconcile: func() reconcile.Stats { return reconcile.Stats{} }})

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/control-plane-health", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"telemetry_size"`) || !strings.Contains(rec.Body.String(), `"loops"`) {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/control-plane-health/fix", `{"fix":"telemetry_prune"}`))
	if rec.Code != http.StatusOK || tel.compacts != 1 {
		t.Fatalf("fix = %d, compacts %d, body %s", rec.Code, tel.compacts, rec.Body.String())
	}
}
