package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_GapFill2(t *testing.T) {
	var gotMethod, gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotURI = r.Method, r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/databases/main/clone-restores":
			_, _ = w.Write([]byte(`[{"id":"clr_1","new_database_name":"main-copy","backup_history_id":"bkh_1","status":"failed","error":"disk full","started_at":"t"}]`))
		case "/api/v1/apps/web/volumes/data/clone-restores":
			_, _ = w.Write([]byte(`[{"id":"vcr_1","new_volume_name":"clone-web-data","backup_history_id":"bkh_2","status":"succeeded","started_at":"t"}]`))
		case "/api/v1/databases/main/status":
			_, _ = w.Write([]byte(`[{"type":"Ready","status":"False","reason":"ContainerMissing","message":"not running"}]`))
		case "/api/v1/deploys/failed":
			_, _ = w.Write([]byte(`[{"id":"dep_1","service_name":"web","image":"web:2","status":"failed","started_at":"2026-09-01T00:00:00Z","error":"boom","last_good_image":"web:1"}]`))
		case "/api/v1/apps/web/deploys/dep_1/steps":
			_, _ = w.Write([]byte("data: {\"step\":\"building\",\"status\":\"running\",\"timestamp\":\"t1\"}\n\ndata: {\"step\":\"building\",\"status\":\"failed\",\"timestamp\":\"t2\"}\n\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantURI  string
		wantOut  string
	}{
		{"backups clone-restores", []string{"backups", "clone-restores", "main"}, exitOK, "/api/v1/databases/main/clone-restores", "main-copy"},
		{"volume clone-restores", []string{"app-volume-backups", "clone-restores", "web", "data"}, exitOK, "/api/v1/apps/web/volumes/data/clone-restores", "clone-web-data"},
		{"databases status", []string{"databases", "status", "main"}, exitOK, "/api/v1/databases/main/status", "ContainerMissing"},
		{"deploys failed", []string{"apps", "deploys", "failed", "--since", "6h"}, exitOK, "/api/v1/deploys/failed?since=6h", "web:1"},
		{"deploys failed json", []string{"apps", "deploys", "failed", "--json"}, exitOK, "/api/v1/deploys/failed", `"last_good_image": "web:1"`},
		{"deploys failed query", []string{"apps", "deploys", "failed", "--json", "--query", "[].service_name"}, exitOK, "/api/v1/deploys/failed", `"web"`},
		{"deploys steps failed exits nonzero", []string{"apps", "deploys", "steps", "web", "dep_1"}, exitAPIError, "/api/v1/apps/web/deploys/dep_1/steps", "failed"},
		{"deploys steps json", []string{"apps", "deploys", "steps", "web", "dep_1", "--json"}, exitAPIError, "/api/v1/apps/web/deploys/dep_1/steps", `"step":"building"`},
		{"missing arg", []string{"backups", "clone-restores"}, exitUsage, "", ""},
		{"bad since", []string{"apps", "deploys", "failed", "--since", "soon"}, exitValidation, "", ""},
		{"api error", []string{"databases", "status", "nope"}, exitAPIError, "/api/v1/databases/nope/status", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMethod, gotURI = "", ""
			var stdout, stderr bytes.Buffer
			args := append(append([]string{}, tt.args...), "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if tt.wantURI != "" && (gotMethod != http.MethodGet || gotURI != tt.wantURI) {
				t.Errorf("request = %s %s, want GET %s", gotMethod, gotURI, tt.wantURI)
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

func TestDeployStepsTerminal(t *testing.T) {
	tests := []struct {
		ev   deployStepEvent
		want bool
	}{
		{deployStepEvent{Step: "building", Status: "running"}, false},
		{deployStepEvent{Step: "pushing", Status: "done"}, false},
		{deployStepEvent{Step: "deploying", Status: "done"}, true},
		{deployStepEvent{Step: "detecting", Status: "failed"}, true},
	}
	for _, tt := range tests {
		if got := deployStepsTerminal(tt.ev); got != tt.want {
			t.Errorf("deployStepsTerminal(%+v) = %v, want %v", tt.ev, got, tt.want)
		}
	}
}
