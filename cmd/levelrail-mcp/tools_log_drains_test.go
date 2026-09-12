package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetAppLogDrain(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/log-drain" {
			t.Errorf("path = %q, want /api/v1/apps/web/log-drain", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.LogDrainResource{AppName: "web", Type: "http", Target: "https://logs.example.com/ingest", Enabled: true})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_log_drain", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_log_drain) error = %v", err)
	}
	var drain apiclient.LogDrainResource
	decodeStructured(t, result, &drain)
	if drain.Type != "http" || !drain.Enabled {
		t.Errorf("drain = %+v, want enabled http drain", drain)
	}
}

func TestGetAppLogDrain_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app has no log drain configured"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_log_drain", Arguments: map[string]any{"name": "web"}})
	if err != nil {
		t.Fatalf("CallTool(get_app_log_drain) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "app has no log drain configured") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}
