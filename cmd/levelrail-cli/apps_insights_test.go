package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps-summary" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(apiclient.AppsSummary{Total: 5, Running: 3, Failing: 1, Stopped: 1})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "summary", "--api-url", srv.URL})
	if !strings.Contains(stdout, "total 5: running 3, failing 1") {
		t.Errorf("stdout = %q", stdout)
	}
	jsonOut, _ := runCLIExpectOK(t, []string{"apps", "summary", "--json", "--api-url", srv.URL})
	var got apiclient.AppsSummary
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil || got.Total != 5 {
		t.Errorf("json = %q err = %v", jsonOut, err)
	}
}

func TestRun_AppsChanges(t *testing.T) {
	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"app":"web","total":2,"changes":[
			{"at":"2026-09-25T10:00:00Z","kind":"env","actor":"gagan","title":"Environment changed","keys":["API_URL"],"likely_cause":true},
			{"at":"2026-09-25T09:00:00Z","kind":"deploy","actor":"manual","title":"Deploy web:2"}]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "changes", "web", "--window", "2h", "--api-url", srv.URL})
	if gotURI != "/api/v1/apps/web/changes?window=2h" {
		t.Errorf("request = %s", gotURI)
	}
	for _, want := range []string{"Environment changed (API_URL)", "yes", "Deploy web:2"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}

func TestRun_AppsDeploysProbes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/deploys/dpl_1/probes" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]apiclient.ProbeAttempt{
			{ID: 1, Target: "http://c:3000/healthz", Success: false, StatusCode: 503, LatencyMS: 12, ProbedAt: "2026-09-25T10:00:00Z"},
			{ID: 2, Target: "http://c:3000/healthz", Success: true, StatusCode: 200, LatencyMS: 8, ProbedAt: "2026-09-25T10:00:05Z"},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "deploys", "probes", "web", "dpl_1", "--api-url", srv.URL})
	for _, want := range []string{"fail", "503", "ok", "200", "12ms"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %s", want, stdout)
		}
	}
}
