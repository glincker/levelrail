package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
