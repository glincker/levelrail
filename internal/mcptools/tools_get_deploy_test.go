package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestGetDeployReturnsFailureObject(t *testing.T) {
	var gotPath string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DeployAttemptResource{
			ID: "dep_2", ServiceName: "web", Status: "failed",
			Failure: &apiclient.DeployFailure{Code: "dockerfile_error", Cause: "c", SuggestedFix: "f", DocsURL: "/deploy-failures#dockerfile_error", DeployID: "dep_2", App: "web"},
		})
	})
	for _, tc := range []struct {
		args     map[string]any
		wantPath string
	}{
		{map[string]any{"name": "web", "deploy_id": "dep_2"}, "/api/v1/apps/web/deploys/dep_2"},
		{map[string]any{"name": "web"}, "/api/v1/apps/web/deploys/latest"},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_deploy", Arguments: tc.args})
		if err != nil {
			t.Fatal(err)
		}
		var got apiclient.DeployAttemptResource
		decodeStructured(t, result, &got)
		if gotPath != tc.wantPath || got.Failure == nil || got.Failure.Code != "dockerfile_error" {
			t.Fatalf("path %q got %+v", gotPath, got)
		}
	}
}
