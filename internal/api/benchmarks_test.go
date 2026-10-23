package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/GLINCKER/levelrail/internal/chaos"
	"github.com/GLINCKER/levelrail/internal/store"
)

func benchReportJSON(t *testing.T, commit string, rps float64, at time.Time) string {
	t.Helper()
	res := chaos.Result{Scenario: chaos.IngressConcurrency, Status: chaos.StatusPassed, RanAt: at}
	res.Set("achieved_rps", rps)
	rep := benchreport.New([]chaos.Result{res}, benchreport.Options{
		Build: benchreport.Build{Version: "v1", Commit: commit},
		Host:  benchreport.Host{OS: "darwin", Arch: "arm64", CPUs: 8}, Repeat: 1, Now: at,
	})
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBenchmarkRunsAPI(t *testing.T) {
	t.Setenv(envBenchmarkKeep, "2")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	serve := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	now := time.Now().UTC().Truncate(time.Second)

	var first, second store.BenchmarkRunSummary
	rec := serve(http.MethodPost, "/api/v1/benchmarks", benchReportJSON(t, "aaa", 1000, now.Add(-2*time.Hour)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &first)
	if !strings.HasPrefix(first.ID, "bench_") || first.Commit != "aaa" || first.Passed != 1 {
		t.Fatalf("summary = %+v", first)
	}
	if rec = serve(http.MethodPost, "/api/v1/benchmarks", benchReportJSON(t, "aaa", 1000, now.Add(-2*time.Hour))); rec.Code != http.StatusOK {
		t.Fatalf("re-upload = %d, want 200 for the same file", rec.Code)
	}
	rec = serve(http.MethodPost, "/api/v1/benchmarks", benchReportJSON(t, "bbb", 1500, now.Add(-time.Hour)))
	_ = json.Unmarshal(rec.Body.Bytes(), &second)

	var list benchmarkListResponse
	_ = json.Unmarshal(serve(http.MethodGet, "/api/v1/benchmarks", "").Body.Bytes(), &list)
	if len(list.Runs) != 2 || list.Runs[0].ID != second.ID || list.Keep != 2 {
		t.Fatalf("list = %+v", list)
	}

	rec = serve(http.MethodGet, "/api/v1/benchmarks/"+first.ID, "")
	var one benchmarkRunResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &one)
	if rec.Code != http.StatusOK || len(one.Report.Results) != 1 || one.Groups[chaos.IngressConcurrency] != benchreport.GroupThroughput || len(one.Report.Caveats) == 0 {
		t.Fatalf("get = %d %+v", rec.Code, one)
	}

	rec = serve(http.MethodGet, "/api/v1/benchmarks/compare?base="+first.ID+"&head="+second.ID, "")
	var cmp benchreport.Comparison
	_ = json.Unmarshal(rec.Body.Bytes(), &cmp)
	var found bool
	for _, d := range cmp.Deltas {
		if d.Metric == "achieved_rps" && d.Direction == benchreport.DirectionBetter && *d.DeltaPct == 50 {
			found = true
		}
	}
	if rec.Code != http.StatusOK || !found || len(cmp.Warnings) == 0 {
		t.Fatalf("compare = %d %s", rec.Code, rec.Body.String())
	}

	third := serve(http.MethodPost, "/api/v1/benchmarks", benchReportJSON(t, "ccc", 2000, now))
	if third.Code != http.StatusCreated {
		t.Fatalf("third upload = %d", third.Code)
	}
	if rec = serve(http.MethodGet, "/api/v1/benchmarks/"+first.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("oldest run should be pruned by retention, got %d", rec.Code)
	}

	if rec = serve(http.MethodDelete, "/api/v1/benchmarks/"+second.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if rec = serve(http.MethodDelete, "/api/v1/benchmarks/"+second.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
}

func TestBenchmarkUploadRejectsBadFiles(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for name, body := range map[string]string{
		"not json":      `nope`,
		"empty":         `{}`,
		"wrong version": strings.Replace(benchReportJSON(t, "aaa", 1, time.Now()), `"schema_version":1`, `"schema_version":9`, 1),
		"no commit":     strings.Replace(benchReportJSON(t, "aaa", 1, time.Now()), `"commit":"aaa"`, `"commit":""`, 1),
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/benchmarks", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: upload = %d, want 400", name, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/benchmarks/compare?base=x", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("compare without head = %d", rec.Code)
	}
}
