package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCloneApp(t *testing.T) {
	var gotBody map[string]string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/clone" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/clone", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: gotBody["new_name"], Image: "nginx:1", Port: 80})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "clone_app",
		Arguments: map[string]any{"name": "web", "new_name": "web-copy"},
	})
	if err != nil {
		t.Fatalf("CallTool(clone_app) error = %v", err)
	}
	var app apiclient.AppResource
	decodeStructured(t, result, &app)
	if app.Name != "web-copy" {
		t.Errorf("app.Name = %q, want web-copy", app.Name)
	}
	if gotBody["new_name"] != "web-copy" {
		t.Errorf("request body new_name = %q, want web-copy", gotBody["new_name"])
	}
}

func TestCloneApp_Conflict(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"an app with this name already exists"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "clone_app",
		Arguments: map[string]any{"name": "web", "new_name": "web"},
	})
	if err != nil {
		t.Fatalf("CallTool(clone_app) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 409 response")
	}
	if got := toolResultText(result); !strings.Contains(got, "already exists") {
		t.Errorf("error text = %q, want the server's conflict message", got)
	}
}

func TestListAppImages(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/images" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/images", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ImageResource{
			{Tag: "abc123", CreatedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_app_images",
		Arguments: map[string]any{"name": "web"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_app_images) error = %v", err)
	}
	var images []apiclient.ImageResource
	decodeStructured(t, result, &images)
	if len(images) != 1 || images[0].Tag != "abc123" {
		t.Errorf("images = %+v, want one image tagged abc123", images)
	}
}

func TestListAppImages_Empty(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.ImageResource{})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_app_images",
		Arguments: map[string]any{"name": "web"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_app_images) error = %v", err)
	}
	var images []apiclient.ImageResource
	decodeStructured(t, result, &images)
	if len(images) != 0 {
		t.Errorf("images = %+v, want empty", images)
	}
}
