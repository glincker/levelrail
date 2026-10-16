package models

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

func llamaStub(t *testing.T, delay time.Duration, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		time.Sleep(delay)
		for i := 0; i < 5; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"tok%d\"}}]}\n\n", i)
			w.(http.Flusher).Flush()
			time.Sleep(2 * time.Millisecond)
		}
		_, _ = fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":40,\"completion_tokens\":5},\"timings\":{\"prompt_n\":40,\"predicted_n\":5,\"prompt_per_second\":88.4,\"predicted_per_second\":6.6}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func benchService(t *testing.T, dial string, cfg BenchConfig, stats BenchStatsFunc) *Service {
	t.Helper()
	svc, db := newSvc(t, nil)
	ctx := context.Background()
	m := store.Model{Name: "bench", Engine: EngineLlamaCpp, ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", Compute: ComputeCPU, CPUMilli: 2000, MemoryMiB: 4096,
		APIKeyHash: "h", APIKeyPrefix: "p", Residency: store.ResidencyAlways}
	if err := db.SaveModel(ctx, m); err != nil {
		t.Fatal(err)
	}
	if err := db.SetModelEndpoint(ctx, "bench", dial); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertConditions(ctx, ControllerName("bench"), []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: "ModelLoaded"}}); err != nil {
		t.Fatal(err)
	}
	svc.SetBenchmark(cfg, nil, stats)
	return svc
}

func TestBenchmark_NotReadyNeverWakes(t *testing.T) {
	svc, db := newSvc(t, nil)
	svc.SetBenchmark(BenchConfig{Timeout: time.Second, MinInterval: time.Minute, MaxTokens: 8, SampleEvery: time.Millisecond}, nil, nil)
	_ = db.SaveModel(context.Background(), store.Model{Name: "idle", Engine: EngineLlamaCpp, ModelRef: "a/b", Compute: ComputeCPU, APIKeyHash: "h", APIKeyPrefix: "p"})
	if _, err := svc.Benchmark(context.Background(), "idle"); !errors.Is(err, ErrBenchNotReady) {
		t.Fatalf("err = %v, want ErrBenchNotReady", err)
	}
}

func TestBenchmark_RunsStoresAndRateLimits(t *testing.T) {
	var hits atomic.Int32
	srv := llamaStub(t, 0, &hits)
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	var peak atomic.Uint64
	peak.Store(1 << 30)
	svc := benchService(t, dial, BenchConfig{Timeout: 5 * time.Second, MinInterval: time.Hour, MaxTokens: 8, SampleEvery: time.Millisecond},
		func(context.Context, store.Model) (uint64, uint64, bool) { return peak.Load(), 4 << 30, true })
	ctx := context.Background()

	res, err := svc.Benchmark(ctx, "bench")
	if err != nil {
		t.Fatalf("Benchmark: %v", err)
	}
	if len(res.Prompts) != len(benchPrompts) || res.GenTPS != 6.6 || res.PromptTPS != 88.4 || res.TTFTMillis < 0 {
		t.Fatalf("result = %+v", res)
	}
	if res.PeakMemoryBytes != 1<<30 || res.MemoryLimitBytes != 4<<30 {
		t.Errorf("memory = %d / %d", res.PeakMemoryBytes, res.MemoryLimitBytes)
	}
	if res.Truncated {
		t.Error("run should not be truncated")
	}
	v, _ := svc.Get(ctx, "bench")
	stored := ParseBenchmark(v.Model.BenchmarkJSON)
	if stored == nil || stored.GenTPS != 6.6 {
		t.Fatalf("stored = %+v", stored)
	}

	before := hits.Load()
	_, err = svc.Benchmark(ctx, "bench")
	var limited *BenchRateLimitError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("second run err = %v, want rate limit", err)
	}
	if hits.Load() != before {
		t.Error("a rate limited benchmark must not touch the engine")
	}
}

func TestBenchmark_HardTimeCapReturnsPartialOrError(t *testing.T) {
	var hits atomic.Int32
	srv := llamaStub(t, 400*time.Millisecond, &hits)
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	svc := benchService(t, dial, BenchConfig{Timeout: 700 * time.Millisecond, MinInterval: time.Nanosecond, MaxTokens: 8, SampleEvery: time.Millisecond}, nil)
	start := time.Now()
	res, err := svc.Benchmark(context.Background(), "bench")
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("benchmark ran %s, the hard cap was ignored", elapsed)
	}
	if err != nil {
		if !strings.Contains(err.Error(), "no result") {
			t.Fatalf("err = %v", err)
		}
		return
	}
	if !res.Truncated || len(res.Prompts) >= len(benchPrompts) {
		t.Fatalf("expected a truncated partial result, got %+v", res)
	}
}

func TestBenchmark_OnlyOneAtATime(t *testing.T) {
	srv := llamaStub(t, 200*time.Millisecond, nil)
	defer srv.Close()
	dial := strings.TrimPrefix(srv.URL, "http://")
	svc := benchService(t, dial, BenchConfig{Timeout: 5 * time.Second, MinInterval: time.Nanosecond, MaxTokens: 8, SampleEvery: time.Millisecond}, nil)
	var wg sync.WaitGroup
	var busy atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Benchmark(context.Background(), "bench"); errors.Is(err, ErrBenchBusy) {
				busy.Add(1)
			}
		}()
		time.Sleep(30 * time.Millisecond)
	}
	wg.Wait()
	if busy.Load() != 1 {
		t.Fatalf("busy rejections = %d, want exactly 1", busy.Load())
	}
}

func TestBenchmark_EngineError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", 500) }))
	defer srv.Close()
	svc := benchService(t, strings.TrimPrefix(srv.URL, "http://"), BenchConfig{Timeout: time.Second, MinInterval: time.Nanosecond, MaxTokens: 8, SampleEvery: time.Millisecond}, nil)
	if _, err := svc.Benchmark(context.Background(), "bench"); err == nil {
		t.Fatal("an engine that fails every prompt must surface an error")
	}
}
