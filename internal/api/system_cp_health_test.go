package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCPDB struct{ checkpoints, optimizes, resets int }

func (f *fakeCPDB) Metrics() store.DBMetrics { return store.DBMetrics{WALBytes: 200 << 20, P95MS: 1} }
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
