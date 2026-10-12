package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SystemRestart(t *testing.T) {
	oldInterval := restartPollInterval
	restartPollInterval = time.Millisecond
	defer func() { restartPollInterval = oldInterval }()

	tests := []struct {
		name       string
		args       []string
		supported  bool
		dropOnPoll bool
		wantExit   int
		wantPosts  int32
		wantOut    string
	}{
		{name: "unsupported prints hint and fails", args: []string{"--yes"}, supported: false, wantExit: exitValidation, wantPosts: 0, wantOut: "sudo systemctl restart levelrail"},
		{name: "no terminal and no --yes refuses", args: nil, supported: true, wantExit: exitValidation, wantPosts: 0, wantOut: "pass --yes"},
		{name: "yes posts and returns", args: []string{"--yes"}, supported: true, wantExit: exitOK, wantPosts: 1, wantOut: "boot id:   old"},
		{name: "wait polls until boot id changes", args: []string{"--yes", "--wait"}, supported: true, dropOnPoll: true, wantExit: exitOK, wantPosts: 1, wantOut: "boot id:   new"},
		{name: "wait times out", args: []string{"--yes", "--wait", "--timeout", "20ms"}, supported: true, wantExit: exitNetwork, wantPosts: 1, wantOut: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var posts, polls int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				res := apiclient.SystemRestart{BootID: "old", Supported: tt.supported}
				if !tt.supported {
					res.Reason = "no process supervisor was detected"
					res.Hint = "sudo systemctl restart levelrail"
				}
				if r.Method == http.MethodPost {
					atomic.AddInt32(&posts, 1)
					res.Pending = true
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(res)
					return
				}
				// The first GET is the pre-flight check; later ones are the wait loop.
				n := atomic.AddInt32(&polls, 1)
				if tt.dropOnPoll && n == 2 {
					if hj, ok := w.(http.Hijacker); ok {
						if conn, _, err := hj.Hijack(); err == nil {
							_ = conn.Close()
						}
					}
					return
				}
				if tt.dropOnPoll && n >= 3 {
					res.BootID = "new"
				}
				_ = json.NewEncoder(w).Encode(res)
			}))
			defer srv.Close()

			var out, errOut bytes.Buffer
			args := append([]string{"system", "restart"}, tt.args...)
			args = append(args, "--api-url", srv.URL)
			got := run("levelrail-cli-test", args, &out, &errOut, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, out.String(), errOut.String())
			}
			if p := atomic.LoadInt32(&posts); p != tt.wantPosts {
				t.Errorf("posts = %d, want %d", p, tt.wantPosts)
			}
			if combined := out.String() + errOut.String(); !strings.Contains(combined, tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", combined, tt.wantOut)
			}
		})
	}
}

func TestRun_SystemRestartStatus(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.SystemRestart{BootID: "abc", Supported: true})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	got := run("levelrail-cli-test", []string{"system", "restart", "status", "--api-url", srv.URL}, &out, &errOut, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, errOut.String())
	}
	if path != "/api/v1/system/restart" || !strings.Contains(out.String(), "boot id:   abc") {
		t.Errorf("path = %q, stdout = %q", path, out.String())
	}
}
