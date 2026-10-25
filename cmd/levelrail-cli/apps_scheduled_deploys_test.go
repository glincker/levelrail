package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_AppsDeployAt(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantImage string
		wantRef   string
		wantErr   string
	}{
		{name: "image", args: []string{"--image", "img:v2", "--at", "in 2h"}, wantImage: "img:v2"},
		{name: "git ref", args: []string{"--latest", "--branch", "release", "--at", "2h"}, wantRef: "release"},
		{name: "latest needs branch", args: []string{"--latest", "--at", "2h"}, wantErr: "--branch"},
		{name: "neither", args: []string{"--at", "2h"}, wantErr: "exactly one"},
		{name: "commit unsupported", args: []string{"--commit", "abc", "--at", "2h"}, wantErr: "exactly one"},
		{name: "bad time", args: []string{"--image", "img:v2", "--at", "tomorrow"}, wantErr: "--at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got apiclient.ScheduledDeployRequest
			var idem string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/scheduled-deploys" {
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
				idem = r.Header.Get("Idempotency-Key")
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(apiclient.ScheduledDeploy{ID: "sdep_1", ServiceName: "web", Kind: "image", Image: got.Image, Ref: got.Ref, RunAt: time.Now().Add(2 * time.Hour), Status: "pending"})
			}))
			defer srv.Close()

			args := append([]string{"apps", "deploy", "web"}, tc.args...)
			args = append(args, "--api-url", srv.URL, "--json")
			var stdout, stderr bytes.Buffer
			code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if tc.wantErr != "" {
				if code == exitOK || !strings.Contains(stdout.String()+stderr.String(), tc.wantErr) {
					t.Fatalf("code = %d, output %q %q, want error containing %q", code, stdout.String(), stderr.String(), tc.wantErr)
				}
				return
			}
			if code != exitOK {
				t.Fatalf("exit = %d (%q %q)", code, stdout.String(), stderr.String())
			}
			if got.Image != tc.wantImage || got.Ref != tc.wantRef || idem == "" {
				t.Errorf("request = %+v idem=%q", got, idem)
			}
			if d := time.Until(mustParse(t, got.RunAt)); d < 119*time.Minute || d > 121*time.Minute {
				t.Errorf("run_at is %v away, want about 2h", d)
			}
		})
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("run_at %q: %v", s, err)
	}
	return at
}

func TestRun_AppsScheduleListAndCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/scheduled-deploys":
			if r.URL.Query().Get("status") != "pending" {
				t.Errorf("status query = %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]apiclient.ScheduledDeploy{{ID: "sdep_1", Kind: "image", Image: "img:v2", Status: "pending", RunAt: time.Now().Add(time.Hour)}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/web/scheduled-deploys/sdep_1":
			_ = json.NewEncoder(w).Encode(apiclient.ScheduledDeploy{ID: "sdep_1", Status: "canceled"})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"apps", "schedule", "list", "web", "--status", "pending", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitOK || !strings.Contains(stdout.String(), "sdep_1") {
		t.Fatalf("list exit = %d out=%q err=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := run("levelrail-cli-test", []string{"apps", "schedule", "cancel", "web", "sdep_1", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap()); code != exitOK || !strings.Contains(stdout.String(), `"canceled"`) {
		t.Fatalf("cancel exit = %d out=%q err=%q", code, stdout.String(), stderr.String())
	}
}
