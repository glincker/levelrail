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

func TestRunAppsDiagnose_FixOutcomes(t *testing.T) {
	missingEnv := diagnosisResource{Confidence: "high", Causes: []apiclient.DiagnosisCause{{
		Code: "MISSING_ENV",
		Fixes: []apiclient.DiagnosisFix{{N: 1, Kind: "input", Label: "Set DATABASE_URL", Redeploy: true,
			Changes: []apiclient.DiagnosisChange{{Field: "env.DATABASE_URL", NeedsInput: true}}}},
	}}}
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantOut    []string
		wantErr    []string
		wantCalls  []string
		wantNoCall string
	}{
		{
			name:       "missing value names the env set command and changes nothing",
			args:       []string{"--apply-fix", "1", "--redeploy"},
			wantExit:   exitValidation,
			wantErr:    []string{"env set web DATABASE_URL=VALUE --apply", "nothing was changed"},
			wantNoCall: "PUT /api/v1/apps/web",
		},
		{
			name:      "value given applies then redeploys and reports the deploy id",
			args:      []string{"--apply-fix", "1", "--input", "env.DATABASE_URL=postgres://x", "--redeploy"},
			wantExit:  exitOK,
			wantOut:   []string{"applied fix 1", "redeployed web, deploy dep_123"},
			wantCalls: []string{"PUT /api/v1/apps/web", "POST /api/v1/apps/web/deploys"},
		},
		{
			name:       "redeploy without apply-fix is refused",
			args:       []string{"--redeploy"},
			wantExit:   exitValidation,
			wantErr:    []string{"--redeploy needs --apply-fix"},
			wantNoCall: "POST /api/v1/apps/web/deploys",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/diagnose"):
					_ = json.NewEncoder(w).Encode(missingEnv)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/deploys/latest"):
					_, _ = w.Write([]byte(`{"id":"dep_123","service_name":"web","image":"i:1","status":"running"}`))
				case r.Method == http.MethodGet:
					_, _ = w.Write([]byte(`{"name":"web","image":"i:1","port":3000}`))
				case r.Method == http.MethodPut:
					b, _ := io.ReadAll(r.Body)
					_, _ = w.Write(b)
				default:
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"name":"web","image":"i:1"}`))
				}
			}))
			t.Cleanup(srv.Close)

			var outBuf, errBuf strings.Builder
			args := append([]string{"apps", "diagnose", "web", "--api-url", srv.URL}, tc.args...)
			got := run("levelrail-cli-test", args, &outBuf, &errBuf, envMap())
			if got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tc.wantExit, outBuf.String(), errBuf.String())
			}
			for _, s := range tc.wantOut {
				if !strings.Contains(outBuf.String(), s) {
					t.Errorf("stdout %q missing %q", outBuf.String(), s)
				}
			}
			for _, s := range tc.wantErr {
				if !strings.Contains(errBuf.String(), s) {
					t.Errorf("stderr %q missing %q", errBuf.String(), s)
				}
			}
			joined := strings.Join(calls, "\n")
			for _, c := range tc.wantCalls {
				if !strings.Contains(joined, c) {
					t.Errorf("calls %q missing %q", joined, c)
				}
			}
			if tc.wantNoCall != "" && strings.Contains(joined, tc.wantNoCall) {
				t.Errorf("unexpected call %q in %q", tc.wantNoCall, joined)
			}
		})
	}
}
