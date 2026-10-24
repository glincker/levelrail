package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type cleanupCall struct {
	method, path, query, body string
}

// cleanupServer answers "METHOD /path" keys with canned JSON and records every call.
func cleanupServer(t *testing.T, routes map[string]string) (*httptest.Server, *[]cleanupCall) {
	t.Helper()
	var calls []cleanupCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, cleanupCall{r.Method, r.URL.Path, r.URL.RawQuery, strings.TrimSpace(string(b))})
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no route"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const cleanupListBody = `{"targets":[{"id":"audit_log","category":"audit","title":"Audit log","days":90,"mode":"on","origin":"existing","usage":{"rows":42,"bytes":1000},"last_run":{"id":7,"target":"audit_log","started_at":"2026-10-22T10:00:00Z","days":90,"dry_run":false,"rows":3,"bytes":30,"trigger":"scheduled"}},{"id":"auth.sessions","category":"auth","days":0,"mode":"preview","origin":"new","usage":{"rows":5,"bytes":50}}],"scheduler":{"enabled":true,"interval":"6h0m0s"}}`

func TestRun_CleanupRequests(t *testing.T) {
	routes := map[string]string{
		"GET /api/v1/cleanup":                    cleanupListBody,
		"POST /api/v1/cleanup/audit_log/preview": `{"rows":12,"bytes":4096}`,
		"POST /api/v1/cleanup/audit_log/run":     `{"target":"audit_log","days":30,"dry_run":false,"rows":12,"bytes":4096}`,
		"PUT /api/v1/cleanup/audit_log/mode":     `{"id":"audit_log","mode":"on"}`,
		"GET /api/v1/cleanup/runs":               `{"runs":[{"id":7,"target":"audit_log","started_at":"2026-10-22T10:00:00Z","days":90,"dry_run":true,"rows":3,"bytes":30,"trigger":"cli","actor":"root"}]}`,
	}
	srv, calls := cleanupServer(t, routes)

	tests := []struct {
		name      string
		args      []string
		wantCalls []cleanupCall
		wantOut   string
		wantErr   string
		wantExit  int
	}{
		{
			name:      "list table",
			args:      []string{"list"},
			wantCalls: []cleanupCall{{"GET", "/api/v1/cleanup", "", ""}},
			wantOut:   "audit_log",
			wantExit:  exitOK,
		},
		{
			name:      "preview without days uses policy",
			args:      []string{"preview", "audit_log"},
			wantCalls: []cleanupCall{{"POST", "/api/v1/cleanup/audit_log/preview", "", "{}"}},
			wantOut:   "would delete 12 rows",
			wantExit:  exitOK,
		},
		{
			name:      "preview with days",
			args:      []string{"preview", "audit_log", "--days", "30", "--json"},
			wantCalls: []cleanupCall{{"POST", "/api/v1/cleanup/audit_log/preview", "", `{"days":30}`}},
			wantOut:   `"rows": 12`,
			wantExit:  exitOK,
		},
		{
			name:     "preview rejects zero days before any call",
			args:     []string{"preview", "audit_log", "--days", "0"},
			wantExit: exitValidation,
		},
		{
			name:     "preview needs one ID",
			args:     []string{"preview"},
			wantExit: exitUsage,
		},
		{
			name:      "run dry run skips preview and sends dry_run",
			args:      []string{"run", "audit_log", "--dry-run", "--days", "30"},
			wantCalls: []cleanupCall{{"POST", "/api/v1/cleanup/audit_log/run", "", `{"days":30,"dry_run":true}`}},
			wantExit:  exitOK,
		},
		{
			name:      "run without yes previews then refuses",
			args:      []string{"run", "audit_log"},
			wantCalls: []cleanupCall{{"POST", "/api/v1/cleanup/audit_log/preview", "", "{}"}},
			wantErr:   "would delete 12 rows",
			wantExit:  exitValidation,
		},
		{
			name: "run with yes previews then deletes",
			args: []string{"run", "audit_log", "--yes", "--days", "30"},
			wantCalls: []cleanupCall{
				{"POST", "/api/v1/cleanup/audit_log/preview", "", `{"days":30}`},
				{"POST", "/api/v1/cleanup/audit_log/run", "", `{"days":30}`},
			},
			wantOut:  "audit_log deleted 12 rows",
			wantExit: exitOK,
		},
		{
			name:     "run rejects zero days",
			args:     []string{"run", "audit_log", "--yes", "--days", "0"},
			wantExit: exitValidation,
		},
		{
			name:      "mode on",
			args:      []string{"mode", "audit_log", "on"},
			wantCalls: []cleanupCall{{"PUT", "/api/v1/cleanup/audit_log/mode", "", `{"mode":"on"}`}},
			wantOut:   "audit_log mode: on",
			wantExit:  exitOK,
		},
		{
			name:     "mode rejects unknown value before any call",
			args:     []string{"mode", "audit_log", "bogus"},
			wantExit: exitValidation,
		},
		{
			name:      "runs filtered by target and date",
			args:      []string{"runs", "--target", "audit_log", "--since", "2026-10-01"},
			wantCalls: []cleanupCall{{"GET", "/api/v1/cleanup/runs", "since=2026-10-01T00%3A00%3A00Z&target=audit_log", ""}},
			wantOut:   "audit_log",
			wantExit:  exitOK,
		},
		{
			name:      "runs csv",
			args:      []string{"runs", "--format", "csv"},
			wantCalls: []cleanupCall{{"GET", "/api/v1/cleanup/runs", "", ""}},
			wantOut:   "id,target,started_at,finished_at,days,dry_run,rows,bytes,trigger,actor,error\n7,audit_log,2026-10-22T10:00:00Z,,90,true,3,30,cli,root,",
			wantExit:  exitOK,
		},
		{
			name:     "runs rejects bad since before any call",
			args:     []string{"runs", "--since", "yesterday"},
			wantExit: exitValidation,
		},
		{
			name:     "runs rejects unknown format",
			args:     []string{"runs", "--format", "xml"},
			wantExit: exitValidation,
		},
		{
			name:     "unknown subcommand",
			args:     []string{"nope"},
			wantExit: exitUsage,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			*calls = nil
			var stdout, stderr bytes.Buffer
			args := append([]string{"cleanup"}, tc.args...)
			args = append(args, "--token", "root-tok", "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tc.wantExit, stdout.String(), stderr.String())
			}
			if len(*calls) != len(tc.wantCalls) {
				t.Fatalf("calls = %+v, want %+v", *calls, tc.wantCalls)
			}
			for i, want := range tc.wantCalls {
				got := (*calls)[i]
				if got.method != want.method || got.path != want.path || got.body != want.body || got.query != want.query {
					t.Errorf("call %d = %+v, want %+v", i, got, want)
				}
			}
			if tc.wantOut != "" && !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.wantOut)
			}
			if tc.wantErr != "" && !strings.Contains(stderr.String(), tc.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantErr)
			}
		})
	}
}

func TestRun_CleanupListTable(t *testing.T) {
	srv, _ := cleanupServer(t, map[string]string{"GET /api/v1/cleanup": cleanupListBody})
	var stdout, stderr bytes.Buffer
	if code := run("lr", []string{"cleanup", "list", "--token", "t", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"ID", "CATEGORY", "LAST RUN", "auth.sessions", "forever", "never", "2026-10-22T10:00:00Z", "scheduler enabled, interval 6h0m0s"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
}

func TestRun_CleanupDryRunOutput(t *testing.T) {
	srv, _ := cleanupServer(t, map[string]string{
		"POST /api/v1/cleanup/audit_log/run": `{"target":"audit_log","dry_run":true,"rows":12,"bytes":4096,"more_remaining":true}`,
	})
	var stdout, stderr bytes.Buffer
	if code := run("lr", []string{"cleanup", "run", "audit_log", "--dry-run", "--token", "t", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "dry run: would delete 12 rows") || !strings.Contains(stdout.String(), "more rows remain") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRun_CleanupRunFailureExitsNonZero(t *testing.T) {
	srv, _ := cleanupServer(t, map[string]string{
		"POST /api/v1/cleanup/audit_log/run": `{"target":"audit_log","rows":0,"error":"busy"}`,
	})
	var stdout, stderr bytes.Buffer
	args := []string{"cleanup", "run", "audit_log", "--dry-run", "--token", "t", "--api-url", srv.URL}
	if code := run("lr", args, &stdout, &stderr, envMap()); code != exitAPIError {
		t.Errorf("exit = %d, want %d", code, exitAPIError)
	}
	if !strings.Contains(stderr.String(), "busy") {
		t.Errorf("stderr = %q, want the run error", stderr.String())
	}
}

func TestRun_CleanupServerError(t *testing.T) {
	srv, _ := cleanupServer(t, map[string]string{})
	var stdout, stderr bytes.Buffer
	if code := run("lr", []string{"cleanup", "run", "audit_log", "--dry-run", "--token", "t", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitAPIError {
		t.Errorf("exit = %d, want %d", code, exitAPIError)
	}
}
