package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestModelTools_RequestsAndResults(t *testing.T) {
	model := apiclient.ModelResource{Name: "chat", Engine: "ollama", Model: "llama3.1:8b", Status: apiclient.ModelStatusResource{Reason: "Downloading", Message: "12%"}}
	tests := []struct {
		tool       string
		args       map[string]any
		wantMethod string
		wantPath   string
		respond    func(w http.ResponseWriter)
		check      func(t *testing.T, result *mcp.CallToolResult)
	}{
		{
			tool: "list_models", args: map[string]any{}, wantMethod: http.MethodGet, wantPath: "/api/v1/models",
			respond: func(w http.ResponseWriter) { _ = json.NewEncoder(w).Encode([]apiclient.ModelResource{model}) },
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out []apiclient.ModelResource
				decodeStructured(t, r, &out)
				if len(out) != 1 || out[0].Status.Reason != "Downloading" {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "get_model", args: map[string]any{"name": "chat"}, wantMethod: http.MethodGet, wantPath: "/api/v1/models/chat",
			respond: func(w http.ResponseWriter) { _ = json.NewEncoder(w).Encode(model) },
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out apiclient.ModelResource
				decodeStructured(t, r, &out)
				if out.Name != "chat" {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "revoke_model_key", args: map[string]any{"name": "chat", "key_id": "k1"}, wantMethod: http.MethodDelete, wantPath: "/api/v1/models/chat/keys/k1",
			respond: func(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) },
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out modelActionResult
				decodeStructured(t, r, &out)
				if !out.OK {
					t.Error("want ok")
				}
			},
		},
		{
			tool: "list_model_keys", args: map[string]any{"name": "chat"}, wantMethod: http.MethodGet, wantPath: "/api/v1/models/chat/keys",
			respond: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode([]apiclient.ModelKeyResource{{ID: "k1", Name: "ci", KeyPrefix: "lr-12345", Status: "active"}})
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out []apiclient.ModelKeyResource
				decodeStructured(t, r, &out)
				if len(out) != 1 || out[0].KeyPrefix != "lr-12345" || strings.Contains(toolResultText(r), "api_key") {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "preflight_model", args: map[string]any{"repo": "acme/chat-GGUF", "quant": "Q4_K_M"}, wantMethod: http.MethodPost, wantPath: "/api/v1/models/preflight",
			respond: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(apiclient.ModelPreflightResult{Repo: "acme/chat-GGUF", Status: "gated", Message: "gated", NextStep: "add a token"})
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out apiclient.ModelPreflightResult
				decodeStructured(t, r, &out)
				if out.Status != "gated" || out.NextStep == "" {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "list_model_cache", args: map[string]any{}, wantMethod: http.MethodGet, wantPath: "/api/v1/model-cache",
			respond: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(apiclient.ModelCacheReport{UnusedDays: 30, Nodes: []apiclient.ModelCacheNode{{Name: "local", Supported: true}}})
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out apiclient.ModelCacheReport
				decodeStructured(t, r, &out)
				if out.UnusedDays != 30 || len(out.Nodes) != 1 {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "get_model_usage", args: map[string]any{"name": "chat", "since": "6h"}, wantMethod: http.MethodGet, wantPath: "/api/v1/models/chat/usage",
			respond: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(apiclient.ModelUsageReport{Model: "chat", Totals: apiclient.ModelUsageTotals{Requests: 3}, Note: "n"})
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out apiclient.ModelUsageReport
				decodeStructured(t, r, &out)
				if out.Totals.Requests != 3 {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "list_gpu_nodes", args: map[string]any{}, wantMethod: http.MethodGet, wantPath: "/api/v1/gpus",
			respond: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode([]apiclient.GPUNodeResource{{Name: "gpu-1", Present: true, GPUCount: 1, TotalVRAMMiB: 24576, Devices: []apiclient.GPUDeviceResource{}}})
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out []apiclient.GPUNodeResource
				decodeStructured(t, r, &out)
				if len(out) != 1 || out[0].TotalVRAMMiB != 24576 {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "deploy_model", args: map[string]any{"name": "chat", "engine": "ollama", "model": "llama3.1:8b"}, wantMethod: http.MethodPost, wantPath: "/api/v1/models",
			respond: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(apiclient.CreateModelResponse{ModelResource: model, APIKey: "lr-onetime"}) //nolint:gosec // test fixture, not a credential
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out apiclient.CreateModelResponse
				decodeStructured(t, r, &out)
				if out.APIKey != "lr-onetime" {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "delete_model", args: map[string]any{"name": "chat"}, wantMethod: http.MethodDelete, wantPath: "/api/v1/models/chat",
			respond: func(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) },
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out modelActionResult
				decodeStructured(t, r, &out)
				if !out.OK {
					t.Error("want ok")
				}
			},
		},
		{
			tool: "restart_model", args: map[string]any{"name": "chat"}, wantMethod: http.MethodPost, wantPath: "/api/v1/models/chat/restart",
			respond: func(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) },
			check:   func(*testing.T, *mcp.CallToolResult) {},
		},
		{
			tool: "rotate_model_api_key", args: map[string]any{"name": "chat"}, wantMethod: http.MethodPost, wantPath: "/api/v1/models/chat/api-key",
			respond: func(w http.ResponseWriter) {
				_ = json.NewEncoder(w).Encode(apiclient.ModelAPIKeyResource{APIKey: "lr-rotated"}) //nolint:gosec // test fixture, not a credential
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out apiclient.ModelAPIKeyResource
				decodeStructured(t, r, &out)
				if out.APIKey != "lr-rotated" {
					t.Errorf("out = %+v", out)
				}
			},
		},
		{
			tool: "get_model_logs", args: map[string]any{"name": "chat", "since": "30m"}, wantMethod: http.MethodGet, wantPath: "/api/v1/models/chat/logs",
			respond: func(w http.ResponseWriter) {
				_, _ = w.Write([]byte(`{"entries":[{"timestamp":"2026-09-24T10:00:00Z","stream":"stdout","message":"loading"}]}`))
			},
			check: func(t *testing.T, r *mcp.CallToolResult) {
				var out []apiclient.LogEntryResource
				decodeStructured(t, r, &out)
				if len(out) != 1 || out[0].Message != "loading" {
					t.Errorf("out = %+v", out)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			var gotMethod, gotPath string
			session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				tt.respond(w)
			})
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if gotMethod != tt.wantMethod || gotPath != tt.wantPath {
				t.Errorf("request = %s %s, want %s %s", gotMethod, gotPath, tt.wantMethod, tt.wantPath)
			}
			tt.check(t, result)
		})
	}
}

func TestModelTools_ForbiddenSurfacesServerError(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"token lacks the required ability"}`))
	})
	calls := map[string]map[string]any{
		"list_models":    {},
		"list_gpu_nodes": {},
		"deploy_model":   {"name": "a", "engine": "ollama", "model": "m"},
	}
	for tool, args := range calls {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || !result.IsError || !strings.Contains(toolResultText(result), "403") {
			t.Errorf("%s: err=%v result=%+v", tool, err, result)
		}
	}
}
