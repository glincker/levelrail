package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/benchreport"
	"github.com/GLINCKER/levelrail/internal/chaos"
)

func TestBenchReportPlanWritesFiles(t *testing.T) {
	dir := t.TempDir()
	plan := &benchReportPlan{jsonPath: dir + "/r.json", mdPath: dir + "/r.md", repeat: 1, note: "unit test host", probe: benchreport.Probe{GOOS: "linux", GOARCH: "amd64", NumCPU: 4}}
	if !plan.wanted() {
		t.Fatal("plan with paths should be wanted")
	}
	plan.begin(context.Background())
	res := chaos.Result{Scenario: chaos.KillContainer, Status: chaos.StatusPassed, Summary: "ok"}
	var stderr bytes.Buffer
	rep, err := plan.finish(context.Background(), []chaos.Result{res}, envMap(), &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Host.Note != "unit test host" || rep.Build.Version == "" || rep.Build.Commit == "" {
		t.Errorf("report = %+v", rep)
	}
	for _, f := range []string{"r.json", "r.md"} {
		if b, err := os.ReadFile(dir + "/" + f); err != nil || len(b) == 0 { //nolint:gosec // path is under t.TempDir()
			t.Errorf("%s not written: %v", f, err)
		}
	}
	if _, err := (&benchReportPlan{probe: benchreport.Probe{GOOS: "linux", NumCPU: 1}}).finish(context.Background(), nil, envMap(), &stderr); err == nil {
		t.Error("a report with no results must be rejected")
	}
}

func TestBuildIdentityCommitOverride(t *testing.T) {
	b := buildIdentity(func(k string) (string, bool) { return "deadbeef", k == envBenchCommit })
	if b.Commit != "deadbeef" || b.Dirty {
		t.Errorf("build = %+v", b)
	}
	if b := buildIdentity(envMap()); b.Version == "" || b.Commit == "" {
		t.Errorf("build without override = %+v", b)
	}
}

func TestBenchChaosRunScenarioNames(t *testing.T) {
	for _, args := range [][]string{{"chaos", "run"}, {"chaos", "run", "kill-container", "nope"}} {
		var stdout, stderr bytes.Buffer
		if got := runBench("levelrail-cli", args, &stdout, &stderr, envMap()); got != exitValidation {
			t.Errorf("%v: exit = %d, want %d", args, got, exitValidation)
		}
	}
}

func TestBenchChaosRunRepeatValidation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := runBench("levelrail-cli", []string{"chaos", "run", "--repeat", "0", "kill-container"}, &stdout, &stderr, envMap()); got == exitOK {
		t.Errorf("repeat 0 must not succeed, stderr=%q", stderr.String())
	}
}

func TestRunBench_Validation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no subcommand", nil, exitUsage},
		{"missing url", []string{"run"}, exitValidation},
		{"bad rate", []string{"run", "--url", "http://x", "--rate", "0"}, exitValidation},
		{"two scenarios", []string{"run", "--url", "http://x", "--kill-replica-url", "http://y", "--restart-control-plane"}, exitValidation},
		{"drain without target", []string{"run", "--url", "http://x", "--drain-node", "n1"}, exitValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := runBench("levelrail-cli", tt.args, &stdout, &stderr, envMap()); got != tt.want {
				t.Errorf("exit = %d, want %d (stderr=%q)", got, tt.want, stderr.String())
			}
		})
	}
}

func TestRunBench_JSONReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	args := []string{"run", "--url", srv.URL, "--direct-url", srv.URL, "--rate", "50", "--duration", "400ms", "--baseline-duration", "200ms", "--json"}
	if got := runBench("levelrail-cli", args, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d, stderr=%q", got, stderr.String())
	}
	var rep benchReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if rep.Ingress.Sent != 20 || rep.Direct == nil || rep.Overhead == nil {
		t.Errorf("report = %+v", rep)
	}
	if !strings.Contains(stderr.String(), "ingress") {
		t.Errorf("progress should go to stderr, got %q", stderr.String())
	}
}
