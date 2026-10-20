package mcptools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPlanImport(t *testing.T) {
	var got apiclient.ImportPlanRequest
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/imports/plan" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ImportPlan{Source: "image", SuggestedName: "nginx", Deploy: "app"})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "plan_import",
		Arguments: map[string]any{"input": "nginx:1", "port": 80},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out apiclient.ImportPlan
	decodeStructured(t, result, &out)
	if out.Source != "image" || got.Text != "nginx:1" || got.Port != 80 {
		t.Errorf("out = %+v, req = %+v", out, got)
	}
}
