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

func TestGetAppImportPlanIsReadOnlyGet(t *testing.T) {
	var method, path string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppImportView{ID: "appimp-1", States: map[string]int{"staged": 1}, Items: []apiclient.AppImportItem{
			{Name: "web", State: "staged", Remaining: []string{"copy volume data"}, Entry: apiclient.AppImportEntry{Verdict: "ready-with-notes"}},
		}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_import_plan", Arguments: map[string]any{"id": "appimp-1"}})
	if err != nil {
		t.Fatal(err)
	}
	var out appImportPlanOutput
	decodeStructured(t, res, &out)
	if method != http.MethodGet || path != "/api/v1/migration/apps/sessions/appimp-1" {
		t.Errorf("request = %s %s", method, path)
	}
	if len(out.Items) != 1 || out.Items[0].Remaining[0] != "copy volume data" {
		t.Errorf("out = %+v", out)
	}
}

// Staging, verifying and routing need the source token or change what runs,
// so no app import tool may be anything but a read.
func TestNoToolCanApplyAnAppImport(t *testing.T) {
	found := false
	for name, meta := range toolTable {
		if !strings.Contains(name, "app_import") && !strings.Contains(name, "import_apps") {
			continue
		}
		found = true
		if meta.Class != ClassRead {
			t.Errorf("tool %q is %s, app import tools must be read-only", name, meta.Class)
		}
		for _, verb := range []string{"stage", "apply", "route", "verify", "rollback", "create"} {
			if strings.Contains(name, verb) {
				t.Errorf("tool %q suggests it can %s an import", name, verb)
			}
		}
	}
	if !found {
		t.Fatal("get_app_import_plan is not classified")
	}
}
