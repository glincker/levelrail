package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRunAppsPreflight_Human(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, apiclient.PreflightReport{
		Status: "warn",
		Checks: []apiclient.PreflightCheck{
			{ID: "disk", Name: "Disk space", Status: "warn", Reason: "only 1 GiB free", Fix: "Prune unused images."},
			{ID: "env", Name: "Env", Status: "pass", Reason: "ok"},
		},
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "preflight", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/preflight" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(stdout, "only 1 GiB free") || !strings.Contains(stdout, "fix: Prune unused images.") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunAppsDiagnose_ApplyFix(t *testing.T) {
	var putBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/diagnose"):
			_ = json.NewEncoder(w).Encode(diagnosisResource{Confidence: "high", Causes: []apiclient.DiagnosisCause{{
				Code:  "WRONG_PORT",
				Fixes: []apiclient.DiagnosisFix{{N: 1, Kind: "patch", Label: "Change port", Changes: []apiclient.DiagnosisChange{{Field: "port", From: "3000", To: "8080"}}}},
			}}})
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"name":"web","image":"i:1","port":3000}`))
		case r.Method == http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			_, _ = w.Write(b)
		}
	}))
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "diagnose", "web", "--apply-fix", "1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "applied fix 1") || !strings.Contains(putBody, `"port":8080`) {
		t.Errorf("stdout = %q, put = %q", stdout, putBody)
	}
}

func TestRunAppsDiagnose_HumanShowsCauses(t *testing.T) {
	srv := newListEchoServer(t, nil, diagnosisResource{Confidence: "high", Explanation: "x", Suggestion: "y", Causes: []apiclient.DiagnosisCause{{
		Code: "MISSING_ENV", Confidence: "high", Title: "Required environment variable is not set",
		Fixes: []apiclient.DiagnosisFix{{N: 1, Kind: "input", Label: "Set API_KEY", Changes: []apiclient.DiagnosisChange{{Field: "env.API_KEY", NeedsInput: true}}}},
	}}})
	t.Cleanup(srv.Close)
	stdout, _ := runCLIExpectOK(t, []string{"apps", "diagnose", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "MISSING_ENV") || !strings.Contains(stdout, "--input env.API_KEY=VALUE") {
		t.Errorf("stdout = %q", stdout)
	}
}
