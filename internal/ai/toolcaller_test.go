package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// TestToolCaller_InProcess exercises ListTools and Call end to end against a fake REST API, with no subprocess or network.
func TestToolCaller_InProcess(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/settings/ai-control" {
			_, _ = w.Write([]byte(`{"mode":"operate"}`))
			return
		}
		if r.URL.Path == "/api/v1/auth/my-access" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps" {
			t.Errorf("request = %s %s, want GET /api/v1/apps", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"web","image":"web:1"}]`))
	}))
	t.Cleanup(apiSrv.Close)

	client := apiclient.NewClient(apiSrv.URL, "test-token")
	ctx := context.Background()
	caller, err := NewToolCaller(ctx, client)
	if err != nil {
		t.Fatalf("NewToolCaller() error = %v", err)
	}
	t.Cleanup(func() { _ = caller.Close() })

	tools, err := caller.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("ListTools() returned no tools, want the full platform catalog")
	}
	var found bool
	for _, tool := range tools {
		if tool.Name == "list_apps" {
			found = true
			if len(tool.InputSchema) == 0 {
				t.Error("list_apps InputSchema is empty")
			}
		}
	}
	if !found {
		t.Error("ListTools() did not include list_apps")
	}

	result, isError, err := caller.Call(ctx, "list_apps", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Call(list_apps) error = %v", err)
	}
	if isError {
		t.Fatalf("Call(list_apps) isError = true, result = %s", result)
	}
	var page struct {
		Apps []struct {
			Name string `json:"name"`
		} `json:"apps"`
	}
	if err := json.Unmarshal([]byte(result), &page); err != nil {
		t.Fatalf("unmarshal result %q: %v", result, err)
	}
	if len(page.Apps) != 1 || page.Apps[0].Name != "web" {
		t.Errorf("apps = %+v, want one app named web", page.Apps)
	}
}

func TestToolCaller_Call_UnknownTool(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(apiSrv.Close)

	client := apiclient.NewClient(apiSrv.URL, "test-token")
	ctx := context.Background()
	caller, err := NewToolCaller(ctx, client)
	if err != nil {
		t.Fatalf("NewToolCaller() error = %v", err)
	}
	t.Cleanup(func() { _ = caller.Close() })

	_, _, err = caller.Call(ctx, "not_a_real_tool", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("Call() with an unknown tool name, error = nil, want an error")
	}
}
