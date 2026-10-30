package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsDeployOfflineNodeExitsNonZeroWithFix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"node worker-b is offline: bring the node back, or move the app with apps move","code":"node_offline"}`))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "deploy", "web", "--image", "levelrail/web:2", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "worker-b") || !strings.Contains(stderr.String(), "apps move") {
		t.Errorf("stderr = %q, want node name and fix", stderr.String())
	}
}

func TestRun_AppsRestartQueueWhileOfflineSendsOptIn(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "levelrail/web:1", Port: 3000})
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "restart", "web", "--queue-while-offline", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK || gotQuery != "queue_while_offline=true" {
		t.Fatalf("exit = %d query = %q stderr = %q", got, gotQuery, stderr.String())
	}
}
