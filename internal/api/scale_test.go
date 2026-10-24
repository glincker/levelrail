package api

// Scale harness: where the control plane first slows as app count grows.
// Opt in with LR_SCALE=1 (LR_SCALE_N="100,300,1000", LR_SCALE_PROF=<dir> for pprof).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type scaleEndpoint struct{ name, path string }

func scaleEndpoints() []scaleEndpoint {
	return []scaleEndpoint{
		{"apps list", "/api/v1/apps"},
		{"apps summary", "/api/v1/apps-summary"},
		{"attention feed", "/api/v1/attention/feed"},
		{"inbox", "/api/v1/inbox"},
		{"search", "/api/v1/search?q=svc-04&limit=20"},
		{"app overview", "/api/v1/apps/svc-0042"},
	}
}

func scaleSeed(t *testing.T, db *store.DB, n int) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for i := range n {
		name := fmt.Sprintf("svc-%04d", i)
		if err := db.SaveDesiredService(ctx, store.DesiredService{
			Name: name, Image: "img:v1", Port: 3000,
			Domains: []string{name + ".example.test", "www." + name + ".example.test"},
		}); err != nil {
			t.Fatalf("seed app: %v", err)
		}
		for d := range 3 {
			status := store.DeployAttemptStatusSucceeded
			if d == 2 && i%10 == 0 {
				status = store.DeployAttemptStatusFailed
			}
			id := fmt.Sprintf("dep_%s_%d", name, d)
			started := now.Add(-time.Duration(3-d) * time.Hour)
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
				ID: id, ServiceName: name, Image: "img:v" + strconv.Itoa(d), Source: store.DeployAttemptSourceImage,
				Status: store.DeployAttemptStatusRunning, StartedAt: started,
			}); err != nil {
				t.Fatalf("seed deploy: %v", err)
			}
			errMsg := ""
			if status == store.DeployAttemptStatusFailed {
				errMsg = "synthetic failure"
			}
			if err := db.FinishDeployAttempt(ctx, id, status, started.Add(time.Minute), errMsg); err != nil {
				t.Fatalf("finish deploy: %v", err)
			}
		}
		if err := db.UpsertConditions(ctx, "app/"+name, scaleConditions("ok")); err != nil {
			t.Fatalf("seed conditions: %v", err)
		}
	}
}

func scaleConditions(msg string) []reconcile.Condition {
	return []reconcile.Condition{
		{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running", Message: msg},
		{Type: "Routed", Status: reconcile.ConditionTrue, Reason: "Synced", Message: msg},
		{Type: "Healthy", Status: reconcile.ConditionTrue, Reason: "Probe", Message: msg},
	}
}

type scaleStats struct{ p50, p95, p99, max time.Duration }

func scaleSummarize(d []time.Duration) scaleStats {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	at := func(p int) time.Duration { return d[min(len(d)-1, len(d)*p/100)] }
	return scaleStats{at(50), at(95), at(99), d[len(d)-1]}
}

func scaleGet(h http.Handler, tok, path string) (time.Duration, int) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	start := time.Now()
	h.ServeHTTP(rec, req)
	return time.Since(start), rec.Code
}

func scaleRun(h http.Handler, tok, path string, total, workers int) []time.Duration {
	out := make([]time.Duration, 0, total)
	var mu sync.Mutex
	var wg sync.WaitGroup
	per := total / workers
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]time.Duration, 0, per)
			for range per {
				d, _ := scaleGet(h, tok, path)
				local = append(local, d)
			}
			mu.Lock()
			out = append(out, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func scaleHeap() (uint64, int) {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc, runtime.NumGoroutine()
}

func TestScale_ControlPlane(t *testing.T) {
	if testing.Short() || os.Getenv("LR_SCALE") == "" {
		t.Skip("scale harness, set LR_SCALE=1 (skipped under -short)")
	}
	sizes := []int{100, 300, 1000}
	if v := os.Getenv("LR_SCALE_N"); v != "" {
		sizes = nil
		for _, f := range strings.Split(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				t.Fatalf("LR_SCALE_N: %v", err)
			}
			sizes = append(sizes, n)
		}
	}
	for _, n := range sizes {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			rt, db := newTestRouter(t)
			// Handler() rebuilds the mux, so build it once like production does.
			h := rt.Handler()
			seedStart := time.Now()
			scaleSeed(t, db, n)
			tok := seedMatrixToken(t, db, "root", []string{AbilityRoot})
			t.Logf("N=%d seed=%s", n, time.Since(seedStart).Round(time.Millisecond))
			heap0, g0 := scaleHeap()

			var slowest scaleEndpoint
			var slowestP95 time.Duration
			for _, ep := range scaleEndpoints() {
				if _, code := scaleGet(h, tok, ep.path); code != http.StatusOK {
					t.Logf("N=%d %-15s status %d, skipped", n, ep.name, code)
					continue
				}
				for _, c := range []int{1, 8} {
					st := scaleSummarize(scaleRun(h, tok, ep.path, 240, c))
					t.Logf("N=%d %-15s c=%d p50=%s p95=%s p99=%s max=%s", n, ep.name, c, st.p50, st.p95, st.p99, st.max)
					if c == 1 && st.p95 > slowestP95 {
						slowest, slowestP95 = ep, st.p95
					}
				}
			}
			heap1, g1 := scaleHeap()
			t.Logf("N=%d heap %.1f -> %.1f MiB, goroutines %d -> %d", n, float64(heap0)/1048576, float64(heap1)/1048576, g0, g1)

			scaleWriteContention(t, h, db, tok, n)

			if dir := os.Getenv("LR_SCALE_PROF"); dir != "" && n == sizes[len(sizes)-1] && slowest.path != "" {
				scaleProfile(t, h, tok, slowest, dir)
			}
		})
	}
}

// scaleWriteContention times reconcile status upserts alone, then while
// 8 readers hit the apps list, since the store has a single connection.
func scaleWriteContention(t *testing.T, h http.Handler, db *store.DB, tok string, n int) {
	t.Helper()
	write := func(dur time.Duration) []time.Duration {
		var out []time.Duration
		var mu sync.Mutex
		var wg sync.WaitGroup
		deadline := time.Now().Add(dur)
		for w := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; time.Now().Before(deadline); i++ {
					app := fmt.Sprintf("app/svc-%04d", (w*97+i)%n)
					start := time.Now()
					if err := db.UpsertConditions(context.Background(), app, scaleConditions("tick")); err != nil {
						t.Errorf("upsert: %v", err)
						return
					}
					d := time.Since(start)
					mu.Lock()
					out = append(out, d)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		return out
	}
	alone := write(2 * time.Second)
	sa := scaleSummarize(alone)
	t.Logf("N=%d writes alone: %d ops p50=%s p95=%s p99=%s max=%s", n, len(alone), sa.p50, sa.p95, sa.p99, sa.max)

	stop := make(chan struct{})
	var rg sync.WaitGroup
	var reads int64
	var rmu sync.Mutex
	for range 8 {
		rg.Add(1)
		go func() {
			defer rg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				scaleGet(h, tok, "/api/v1/apps")
				rmu.Lock()
				reads++
				rmu.Unlock()
			}
		}()
	}
	mixed := write(2 * time.Second)
	close(stop)
	rg.Wait()
	sm := scaleSummarize(mixed)
	t.Logf("N=%d writes under 8 readers: %d ops (%d reads) p50=%s p95=%s p99=%s max=%s", n, len(mixed), reads, sm.p50, sm.p95, sm.p99, sm.max)
}

func scaleProfile(t *testing.T, h http.Handler, tok string, ep scaleEndpoint, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // dev-supplied profile dir
		t.Fatal(err)
	}
	cpu, err := os.Create(filepath.Join(dir, "cpu.pprof")) //nolint:gosec // dev-supplied profile dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cpu.Close() }()
	if err := pprof.StartCPUProfile(cpu); err != nil {
		t.Fatal(err)
	}
	scaleRun(h, tok, ep.path, 400, 1)
	pprof.StopCPUProfile()
	runtime.GC()
	heap, err := os.Create(filepath.Join(dir, "heap.pprof")) //nolint:gosec // dev-supplied profile dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = heap.Close() }()
	if err := pprof.Lookup("allocs").WriteTo(heap, 0); err != nil {
		t.Fatal(err)
	}
	t.Logf("profiled %s, wrote %s", ep.name, dir)
}
