package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPreviewCloneEnvironment(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/environments/env_1/clone/preview" {
			t.Errorf("request = %s %s, want GET /api/v1/environments/env_1/clone/preview", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("new_environment_name"); got != "staging-eu" {
			t.Errorf("new_environment_name query = %q, want staging-eu", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.EnvironmentClonePreviewResource{
			NewEnvironmentName: "staging-eu",
			Apps: []apiclient.EnvironmentCloneAppPreview{
				{SourceApp: "web", SuggestedNewName: "web-staging-eu", Image: "app:v1"},
			},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "preview_clone_environment",
		Arguments: map[string]any{"environment_id": "env_1", "new_environment_name": "staging-eu"},
	})
	if err != nil {
		t.Fatalf("CallTool(preview_clone_environment) error = %v", err)
	}
	var preview apiclient.EnvironmentClonePreviewResource
	decodeStructured(t, result, &preview)
	if len(preview.Apps) != 1 || preview.Apps[0].SuggestedNewName != "web-staging-eu" {
		t.Errorf("preview = %+v, want one app suggesting web-staging-eu", preview)
	}
}

func TestCloneEnvironment(t *testing.T) {
	var gotBody apiclient.EnvironmentCloneRequest
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/environments/env_1/clone" {
			t.Errorf("request = %s %s, want POST /api/v1/environments/env_1/clone", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.EnvironmentCloneResultResource{
			Environment: apiclient.EnvironmentResource{ID: "env_2", Name: "staging-eu"},
			Apps: []apiclient.EnvironmentCloneAppResult{
				{SourceApp: "web", NewApp: "web-staging-eu", Image: "app:v1"},
			},
			CopiedSecretValues: true,
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "clone_environment",
		Arguments: map[string]any{
			"environment_id":       "env_1",
			"new_environment_name": "staging-eu",
			"copy_secret_values":   true,
		},
	})
	if err != nil {
		t.Fatalf("CallTool(clone_environment) error = %v", err)
	}
	var got apiclient.EnvironmentCloneResultResource
	decodeStructured(t, result, &got)
	if got.Environment.ID != "env_2" || len(got.Apps) != 1 {
		t.Errorf("result = %+v, want environment env_2 with one cloned app", got)
	}
	if !gotBody.CopySecretValues || gotBody.NewEnvironmentName != "staging-eu" {
		t.Errorf("request body = %+v, want copy_secret_values true, new_environment_name staging-eu", gotBody)
	}
}
