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

func TestPreviewPromoteApp(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/promote/preview" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/promote/preview", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("to"); got != "env_1" {
			t.Errorf("to query = %q, want env_1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.PromotePreviewResource{
			SourceApp: "web", TargetApp: "web-staging",
			From: apiclient.PromotePreviewSide{AppName: "web", Image: "app:v2"},
			To:   apiclient.PromotePreviewSide{AppName: "web-staging", Image: "app:v1"},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "preview_promote_app",
		Arguments: map[string]any{"name": "web", "to": "env_1"},
	})
	if err != nil {
		t.Fatalf("CallTool(preview_promote_app) error = %v", err)
	}
	var preview apiclient.PromotePreviewResource
	decodeStructured(t, result, &preview)
	if preview.TargetApp != "web-staging" || preview.To.Image != "app:v1" {
		t.Errorf("preview = %+v, want target web-staging currently on app:v1", preview)
	}
}

func TestPromoteApp(t *testing.T) {
	var gotBody apiclient.PromoteAppRequest
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/promote" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/promote", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web-staging", Image: "app:v2"})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "promote_app",
		Arguments: map[string]any{"name": "web", "to": "env_1", "target": "web-staging"},
	})
	if err != nil {
		t.Fatalf("CallTool(promote_app) error = %v", err)
	}
	var app apiclient.AppResource
	decodeStructured(t, result, &app)
	if app.Name != "web-staging" || app.Image != "app:v2" {
		t.Errorf("app = %+v, want web-staging on app:v2", app)
	}
	if gotBody.To != "env_1" || gotBody.Target != "web-staging" {
		t.Errorf("request body = %+v, want to env_1 target web-staging", gotBody)
	}
}

func TestPromoteApp_ProtectedEnvironmentRequiresConfirm(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"environment is protected; confirm required"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "promote_app",
		Arguments: map[string]any{"name": "web", "to": "env_1"},
	})
	if err != nil {
		t.Fatalf("CallTool(promote_app) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 409 response")
	}
	if got := toolResultText(result); !strings.Contains(got, "confirm required") {
		t.Errorf("error text = %q, want the server's confirmation message", got)
	}
}
