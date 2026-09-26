package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestCancelDeploy(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/deploys/dep_2/cancel" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.DeployAttemptResource{ID: "dep_2", ServiceName: "web", Status: "canceled", CanceledBy: "token:ci"})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "cancel_deploy", Arguments: map[string]any{"name": "web", "deploy_id": "dep_2"}})
	if err != nil {
		t.Fatal(err)
	}
	var got apiclient.DeployAttemptResource
	decodeStructured(t, result, &got)
	if got.Status != "canceled" || got.CanceledBy != "token:ci" {
		t.Fatalf("got %+v", got)
	}
}

func TestCancelDeployConflictSurfacesAsToolError(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"this deploy already cut over"}`))
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "cancel_deploy", Arguments: map[string]any{"name": "web", "deploy_id": "dep_2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(toolResultText(result), "cut over") {
		t.Fatalf("result = %+v", result)
	}
}
