package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestAppsRun(t *testing.T) {
	seven := 7
	tests := []struct {
		name       string
		args       []string
		final      apiclient.OneShotJob
		wantCode   int
		wantApp    string
		wantCmd    string
		wantStdout string
		wantStderr string
		noServer   bool
	}{
		{name: "success prints output", args: []string{"apps", "run", "web", "--", "echo", "hi"},
			final: apiclient.OneShotJob{State: "Succeeded", Output: "hi\n"}, wantApp: "web", wantCmd: "echo hi", wantStdout: "hi\n"},
		{name: "service targets the sibling app", args: []string{"apps", "run", "shop", "--service", "worker", "--", "bin/migrate"},
			final: apiclient.OneShotJob{State: "Succeeded"}, wantApp: "shop-worker", wantCmd: "bin/migrate"},
		{name: "command exit code is returned", args: []string{"apps", "run", "web", "--", "false"},
			final:    apiclient.OneShotJob{State: "Failed", ExitCode: &seven, Output: "boom", Message: "backoff limit 0 exceeded"},
			wantCode: 7, wantApp: "web", wantCmd: "false", wantStdout: "boom\n", wantStderr: "exit 7"},
		{name: "timeout is 124", args: []string{"apps", "run", "web", "--", "sleep", "99"},
			final: apiclient.OneShotJob{State: "Failed", RunStatus: "timeout"}, wantCode: exitRunTimeout, wantApp: "web", wantCmd: "sleep 99"},
		{name: "failure without a code is 5", args: []string{"apps", "run", "web", "--", "x"},
			final: apiclient.OneShotJob{State: "Failed", Message: "container could not start"}, wantCode: exitDeployFailed, wantApp: "web", wantCmd: "x"},
		{name: "command needs the separator", args: []string{"apps", "run", "web", "ls"}, wantCode: exitValidation, noServer: true},
		{name: "command is required", args: []string{"apps", "run", "web"}, wantCode: exitValidation, noServer: true},
		{name: "app is required", args: []string{"apps", "run"}, wantCode: exitUsage, noServer: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotReq apiclient.OneShotJobRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/jobs/runs":
					body, _ := io.ReadAll(r.Body)
					_ = json.Unmarshal(body, &gotReq)
					_ = json.NewEncoder(w).Encode(apiclient.OneShotJob{ID: "jb_1", App: gotReq.App, State: "Running"})
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs/runs/jb_1":
					j := tc.final
					j.ID, j.App = "jb_1", gotReq.App
					_ = json.NewEncoder(w).Encode(j)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			var out, errb bytes.Buffer
			// Flags go before the app so they are not swallowed by the command after --.
			args := append([]string{"apps", "run", "--api-url", srv.URL, "--token", "t", "--poll", "1ms"}, tc.args[2:]...)
			if got := run("levelrail-cli-test", args, &out, &errb, envMap()); got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tc.wantCode, out.String(), errb.String())
			}
			if tc.noServer {
				if gotReq.App != "" {
					t.Errorf("a job was started for %q, want none", gotReq.App)
				}
				return
			}
			if gotReq.App != tc.wantApp || strings.Join(gotReq.Command, " ") != tc.wantCmd {
				t.Errorf("request = %+v, want app %q command %q", gotReq, tc.wantApp, tc.wantCmd)
			}
			if out.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", out.String(), tc.wantStdout)
			}
			if !strings.Contains(errb.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", errb.String(), tc.wantStderr)
			}
		})
	}
}

func TestAppsRun_JSONKeepsStdoutPure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.OneShotJob{ID: "jb_1", App: "web", State: "Succeeded", Output: "hi\n"})
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	args := []string{"apps", "run", "web", "--json", "--api-url", srv.URL, "--token", "t", "--", "echo", "hi"}
	if got := run("levelrail-cli-test", args, &out, &errb, envMap()); got != exitOK {
		t.Fatalf("exit = %d (%s)", got, errb.String())
	}
	var job apiclient.OneShotJob
	if err := json.Unmarshal(out.Bytes(), &job); err != nil || job.Output != "hi\n" {
		t.Fatalf("stdout = %q, want one JSON job with the output (err %v)", out.String(), err)
	}
}
