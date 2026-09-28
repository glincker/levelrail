package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_PipelinesRunsAll(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pipeline-runs" {
			http.NotFound(w, r)
			return
		}
		gotQuery = r.URL.Query().Encode()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.PipelineRunRowsPage{Runs: []apiclient.PipelineRunRow{
			{ID: "r1", App: "web", Pipeline: "ci", Number: 3, Status: "waiting_approval", ApprovalPending: true, Trigger: "push"},
		}})
	}))
	defer srv.Close()

	tests := []struct {
		name      string
		args      []string
		wantExit  int
		wantOut   string
		wantQuery string
	}{
		{"table", []string{"pipelines", "runs", "--all", "--status", "failed", "--app", "web", "--limit", "5", "--api-url", srv.URL}, exitOK, "waiting_approval (approval)", "app=web&limit=5&status=failed"},
		{"json", []string{"pipelines", "runs", "--all", "--json", "--api-url", srv.URL}, exitOK, `"id": "r1"`, "limit=20"},
		{"all with app argument", []string{"pipelines", "runs", "--all", "web", "--api-url", srv.URL}, exitUsage, "", ""},
		{"no app and no all", []string{"pipelines", "runs", "--api-url", srv.URL}, exitUsage, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotQuery = ""
			var stdout, stderr bytes.Buffer
			if got := run("cli", tc.args, &stdout, &stderr, envMap()); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr %q)", got, tc.wantExit, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantOut)
			}
			if gotQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tc.wantQuery)
			}
		})
	}
}
