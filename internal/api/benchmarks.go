package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envBenchmarkKeep     = "APP_BENCHMARK_RUNS_KEEP"
	envBenchmarkNoisePct = "APP_BENCHMARK_COMPARE_NOISE_PCT"

	defaultBenchmarkKeep     = 50
	defaultBenchmarkNoisePct = 5.0
	maxBenchmarkBodyBytes    = 1 << 20
	maxBenchmarkListLimit    = 200
)

// BenchmarkStore persists benchmark results files.
type BenchmarkStore interface {
	RecordBenchmarkRun(ctx context.Context, run store.BenchmarkRun, keep int) (string, bool, error)
	ListBenchmarkRuns(ctx context.Context, limit int) ([]store.BenchmarkRunSummary, error)
	GetBenchmarkRun(ctx context.Context, id string) (store.BenchmarkRun, error)
	DeleteBenchmarkRun(ctx context.Context, id string) error
}

type benchmarkListResponse struct {
	Runs []store.BenchmarkRunSummary `json:"runs"`
	Keep int                         `json:"keep"`
}

type benchmarkRunResponse struct {
	Run    store.BenchmarkRunSummary `json:"run"`
	Report benchreport.Report        `json:"report"`
	Groups map[string]string         `json:"groups"`
}

func benchmarkKeep() int {
	if n, err := strconv.Atoi(os.Getenv(envBenchmarkKeep)); err == nil && n > 0 {
		return n
	}
	return defaultBenchmarkKeep
}

func benchmarkNoisePct() float64 {
	if f, err := strconv.ParseFloat(os.Getenv(envBenchmarkNoisePct), 64); err == nil && f >= 0 {
		return f
	}
	return defaultBenchmarkNoisePct
}

func benchmarkGroups() map[string]string {
	g := map[string]string{}
	for _, info := range chaos.Catalog() {
		g[info.Name] = benchreport.Group(info.Name)
	}
	return g
}

func (rt *Router) benchmarksReady(w http.ResponseWriter) bool {
	if rt.benchmarks == nil {
		writeError(w, http.StatusNotImplemented, "benchmark results are not available on this control plane")
		return false
	}
	return true
}

func (rt *Router) handleListBenchmarks(w http.ResponseWriter, r *http.Request) {
	if !rt.benchmarksReady(w) {
		return
	}
	limit := benchmarkKeep()
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, maxBenchmarkListLimit)
	}
	runs, err := rt.benchmarks.ListBenchmarkRuns(r.Context(), limit)
	if err != nil {
		rt.internalError(w, "api: list benchmark runs", err)
		return
	}
	writeJSON(w, http.StatusOK, benchmarkListResponse{Runs: runs, Keep: benchmarkKeep()})
}

func (rt *Router) handleUploadBenchmark(w http.ResponseWriter, r *http.Request) {
	if !rt.benchmarksReady(w) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBenchmarkBodyBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "results file is too large")
		return
	}
	rep, err := benchreport.Parse(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	canonical, err := json.Marshal(rep)
	if err != nil {
		rt.internalError(w, "api: encode benchmark report", err)
		return
	}
	sum := sha256.Sum256(canonical)
	passed, failed, skipped := rep.Tally()
	_, _, actor, _ := rt.currentActor(r)
	run := store.BenchmarkRun{
		BenchmarkRunSummary: store.BenchmarkRunSummary{
			ID: "bench_" + hex.EncodeToString(sum[:6]), GeneratedAt: rep.GeneratedAt, Version: rep.Build.Version, Commit: rep.Build.Commit,
			Dirty: rep.Build.Dirty, HostSummary: rep.Host.Summary(), HostOS: rep.Host.OS, Repeat: rep.Repeat,
			Passed: passed, Failed: failed, Skipped: skipped, RecordedBy: actor,
		},
		Report: canonical,
	}
	id, created, err := rt.benchmarks.RecordBenchmarkRun(r.Context(), run, benchmarkKeep())
	if err != nil {
		rt.internalError(w, "api: record benchmark run", err)
		return
	}
	stored, err := rt.benchmarks.GetBenchmarkRun(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: read back benchmark run", err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, stored.BenchmarkRunSummary)
}

func (rt *Router) loadBenchmark(w http.ResponseWriter, r *http.Request, id string) (store.BenchmarkRun, benchreport.Report, bool) {
	run, err := rt.benchmarks.GetBenchmarkRun(r.Context(), id)
	if errors.Is(err, store.ErrBenchmarkRunNotFound) {
		writeError(w, http.StatusNotFound, "benchmark run not found")
		return run, benchreport.Report{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get benchmark run", err)
		return run, benchreport.Report{}, false
	}
	var rep benchreport.Report
	if err := json.Unmarshal(run.Report, &rep); err != nil {
		rt.internalError(w, "api: decode stored benchmark report", err)
		return run, rep, false
	}
	return run, rep, true
}

func (rt *Router) handleGetBenchmark(w http.ResponseWriter, r *http.Request) {
	if !rt.benchmarksReady(w) {
		return
	}
	run, rep, ok := rt.loadBenchmark(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, benchmarkRunResponse{Run: run.BenchmarkRunSummary, Report: rep, Groups: benchmarkGroups()})
}

func (rt *Router) handleCompareBenchmarks(w http.ResponseWriter, r *http.Request) {
	if !rt.benchmarksReady(w) {
		return
	}
	baseID, headID := r.URL.Query().Get("base"), r.URL.Query().Get("head")
	if baseID == "" || headID == "" {
		writeError(w, http.StatusBadRequest, "pass base and head run IDs")
		return
	}
	base, baseRep, ok := rt.loadBenchmark(w, r, baseID)
	if !ok {
		return
	}
	head, headRep, ok := rt.loadBenchmark(w, r, headID)
	if !ok {
		return
	}
	side := func(s store.BenchmarkRunSummary) benchreport.Side {
		return benchreport.Side{ID: s.ID, GeneratedAt: s.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), Version: s.Version, Commit: s.Commit, Dirty: s.Dirty, Host: s.HostSummary}
	}
	writeJSON(w, http.StatusOK, benchreport.Compare(baseRep, headRep, side(base.BenchmarkRunSummary), side(head.BenchmarkRunSummary), benchmarkNoisePct()))
}

func (rt *Router) handleDeleteBenchmark(w http.ResponseWriter, r *http.Request) {
	if !rt.benchmarksReady(w) {
		return
	}
	err := rt.benchmarks.DeleteBenchmarkRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrBenchmarkRunNotFound) {
		writeError(w, http.StatusNotFound, "benchmark run not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete benchmark run", err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
