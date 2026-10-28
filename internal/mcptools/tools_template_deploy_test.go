package mcptools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const installBody = `{"app_id":"pg-ab12","services":[{"name":"pg-ab12","image":"postgres:16","domains":["db.example.com"],"secret_env":["POSTGRES_PASSWORD"],"env":{"MODE":"prod","API_TOKEN":"••••••••"},"secrets":{"POSTGRES_PASSWORD":"hunter2-plain-value"}}]}`

func TestDeployTemplateToolRegisteredWithDeployTier(t *testing.T) {
	var found bool
	for _, tool := range listAll(t, Options{Mode: ModeFull, AllowWrite: true}) {
		if tool.Name == "deploy_template" {
			found = true
			if tool.InputSchema == nil {
				t.Error("deploy_template has no input schema")
			}
		}
	}
	if !found {
		t.Fatal("deploy_template not registered")
	}
	for _, tool := range listAll(t, Options{Mode: ModeReadOnly}) {
		if tool.Name == "deploy_template" {
			t.Error("deploy_template must not be listed in read-only mode")
		}
	}
	if _, ok := deployTierTools["deploy_template"]; !ok {
		t.Error("deploy_template must need the deploy ability like deploy_app")
	}
}

func TestDeployTemplateNeverReturnsSecretValues(t *testing.T) {
	var gotPath, gotBody string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, string(b)
		_, _ = w.Write([]byte(installBody))
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "deploy_template", Arguments: map[string]any{
		"id": "postgres", "name": "pg-ab12", "env": map[string]any{"MODE": "prod"},
	}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %s", err, toolResultText(res))
	}
	if gotPath != "/api/v1/service-templates/postgres/install" || !strings.Contains(gotBody, `"name":"pg-ab12"`) || !strings.Contains(gotBody, `"MODE":"prod"`) {
		t.Errorf("request %s %s", gotPath, gotBody)
	}
	text := toolResultText(res)
	raw, _ := json.Marshal(res.StructuredContent)
	all := text + string(raw)
	if strings.Contains(all, "hunter2-plain-value") || strings.Contains(all, "••••") {
		t.Errorf("secret value leaked: %s", all)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("output holds null: %s", raw)
	}
	var out deployTemplateOutput
	decodeStructured(t, res, &out)
	if out.App != "pg-ab12" || len(out.Services) != 1 || out.Services[0].URL != "https://db.example.com" {
		t.Errorf("got %+v", out)
	}
	if strings.Join(out.SecretKeys, ",") != "API_TOKEN,POSTGRES_PASSWORD" || out.Reveal == "" {
		t.Errorf("secret keys %v reveal %q", out.SecretKeys, out.Reveal)
	}
}
