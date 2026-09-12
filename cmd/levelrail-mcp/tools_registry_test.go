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

func TestListRegistryCredentialRepositories(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/registry-credentials/rc1/repositories" {
			t.Errorf("path = %q, want /api/v1/registry-credentials/rc1/repositories", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.RegistryRepositoriesResource{Repositories: []string{"alpha", "beta"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_registry_credential_repositories",
		Arguments: map[string]any{"id": "rc1"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_registry_credential_repositories) error = %v", err)
	}
	var repos apiclient.RegistryRepositoriesResource
	decodeStructured(t, result, &repos)
	if len(repos.Repositories) != 2 || repos.Repositories[0] != "alpha" {
		t.Errorf("repos = %+v, want [alpha beta]", repos)
	}
}

func TestListRegistryCredentialRepositories_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"registry credential not found"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_registry_credential_repositories",
		Arguments: map[string]any{"id": "ghost"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_registry_credential_repositories) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "registry credential not found") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

func TestListRegistryCredentialTags(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/registry-credentials/rc1/tags" {
			t.Errorf("path = %q, want /api/v1/registry-credentials/rc1/tags", r.URL.Path)
		}
		gotQuery = r.URL.Query().Get("repository")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.RegistryTagsResource{Repository: "myapp", Tags: []string{"latest", "v1"}})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_registry_credential_tags",
		Arguments: map[string]any{"id": "rc1", "repository": "myapp"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_registry_credential_tags) error = %v", err)
	}
	var tags apiclient.RegistryTagsResource
	decodeStructured(t, result, &tags)
	if tags.Repository != "myapp" || len(tags.Tags) != 2 {
		t.Errorf("tags = %+v, want repository myapp with 2 tags", tags)
	}
	if gotQuery != "myapp" {
		t.Errorf("repository query param = %q, want myapp", gotQuery)
	}
}

func TestListRegistryCredentialTags_RepositoryNotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"repository not found in this registry"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_registry_credential_tags",
		Arguments: map[string]any{"id": "rc1", "repository": "ghost"},
	})
	if err != nil {
		t.Fatalf("CallTool(list_registry_credential_tags) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 404 response")
	}
	if !strings.Contains(toolResultText(result), "repository not found in this registry") {
		t.Errorf("error text = %q, want it to contain the server's own 404 message", toolResultText(result))
	}
}

// TestRegistryCredentialBrowseTools_Surface403 mirrors
// TestPlatformExpansionTools_Surface403's own shape for these two tools:
// each hits its own distinct API path and could regress independently.
func TestRegistryCredentialBrowseTools_Surface403(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"list_registry_credential_repositories", map[string]any{"id": "rc1"}},
		{"list_registry_credential_tags", map[string]any{"id": "rc1", "repository": "myapp"}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"token lacks the required ability"}`))
			})

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error = %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("IsError = false, want true for a 403 response")
			}
		})
	}
}
