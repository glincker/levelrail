package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_NodesEvents(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]nodeStatusEventResource{
			{FromStatus: "online", ToStatus: "offline", CreatedAt: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "events", "nd_1", "--limit", "5", "--api-url", srv.URL})
	if gotPath != "/api/v1/nodes/nd_1/events" || gotQuery != "limit=5" {
		t.Errorf("request = %s?%s, want /api/v1/nodes/nd_1/events?limit=5", gotPath, gotQuery)
	}
	if !strings.Contains(stdout, "online") || !strings.Contains(stdout, "offline") {
		t.Errorf("stdout = %q, want the transition", stdout)
	}
}

func TestRun_NodesEvents_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "events", "nd_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "No status changes recorded yet.") {
		t.Errorf("stdout = %q", stdout)
	}
}
