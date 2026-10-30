package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// These tests pin the two flows in docs/mcp-recipes.md to the real handlers.
func flowSession(t *testing.T, attempt apiclient.DeployAttemptResource) *mcp.ClientSession {
	t.Helper()
	t.Setenv(EnvWaitInterval, "1")
	return newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"name":"web"}`))
		case r.URL.Path == "/api/v1/apps/web/diagnose":
			_, _ = w.Write([]byte(`{"confidence":"high"}`))
		default:
			_ = json.NewEncoder(w).Encode(attempt)
		}
	})
}

func TestDocsFlowDeployAndConfirmHealthy(t *testing.T) {
	session := flowSession(t, apiclient.DeployAttemptResource{ID: "dep_1", ServiceName: "web", Status: "succeeded", Outcome: "healthy"})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "deploy_app", Arguments: map[string]any{"name": "web", "image": "nginx:1.27", "wait": true}})
	if err != nil {
		t.Fatal(err)
	}
	var got deployAppOutput
	decodeStructured(t, res, &got)
	if got.Wait == nil || got.Wait.Status != "healthy" || got.Wait.DeployID != "dep_1" || got.Wait.Diagnosis != nil {
		t.Fatalf("got %+v", got.Wait)
	}
}

func TestDocsFlowFailureExplained(t *testing.T) {
	session := flowSession(t, apiclient.DeployAttemptResource{
		ID: "dep_2", ServiceName: "web", Status: "failed", Outcome: "failed",
		Failure: &apiclient.DeployFailure{Code: "image_pull_failed", Cause: "manifest unknown", SuggestedFix: "check the tag", LogExcerpt: "pull nginx:bad: manifest unknown"},
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "deploy_app", Arguments: map[string]any{"name": "web", "image": "nginx:bad", "wait": true}})
	if err != nil {
		t.Fatal(err)
	}
	var got deployAppOutput
	decodeStructured(t, res, &got)
	d := got.Wait.Diagnosis
	if got.Wait.Status != "failed" || d == nil || d.Class != "image_pull_failed" || d.Fix != "check the tag" || d.Confidence != "high" || d.Excerpt == "" {
		t.Fatalf("got %+v", got.Wait)
	}
	res, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "wait_for_deploy", Arguments: map[string]any{"name": "web", "deploy_id": "dep_2"}})
	if err != nil {
		t.Fatal(err)
	}
	var again waitForDeployOutput
	decodeStructured(t, res, &again)
	if again.Diagnosis == nil || again.Diagnosis.Class != "image_pull_failed" {
		t.Fatalf("wait_for_deploy %+v", again)
	}
}
