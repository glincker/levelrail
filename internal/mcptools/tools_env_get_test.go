package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetAppEnvListsSecretNamesNotValues(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/secrets") {
			_, _ = w.Write([]byte(`[{"key":"DB_PASS"},{"key":"TOKEN"}]`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "i", "env": map[string]string{"MODE": "prod"}, "secret_env": []string{"TOKEN"}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_env", Arguments: map[string]any{"name": "web"}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %s", err, toolResultText(res))
	}
	var got AppEnvView
	decodeStructured(t, res, &got)
	if got.Env["MODE"] != "prod" || strings.Join(got.SecretKeys, ",") != "DB_PASS,TOKEN" {
		t.Errorf("got %+v", got)
	}
}

func TestAgentCoreProfileOmitsOutputSchemas(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeFull, Profile: ProfileAgentCore})
	if len(tools) == 0 {
		t.Fatal("no tools listed")
	}
	for _, tool := range tools {
		if tool.OutputSchema != nil {
			t.Errorf("%s still advertises an output schema in the agent-core profile", tool.Name)
		}
	}
	full := listAll(t, Options{Mode: ModeFull})
	withSchema := 0
	for _, tool := range full {
		if tool.OutputSchema != nil {
			withSchema++
		}
	}
	if withSchema == 0 {
		t.Error("full mode should keep output schemas")
	}
}
