package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/coolifyexit"
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

func TestGetAppImportExitReportIsReadOnlyGet(t *testing.T) {
	var method, path string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(coolifyexit.Report{Mode: coolifyexit.ModeDryRun, Counts: map[coolifyexit.Outcome]int{coolifyexit.Manual: 1},
			Rows:      []coolifyexit.Row{{Kind: "volume", Name: "web/uploads", Outcome: coolifyexit.Manual, Next: []string{"copy"}}},
			Checklist: []coolifyexit.Step{{ID: "stage", Title: "Stage everything"}}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_app_import_exit_report", Arguments: map[string]any{"id": "appimp-1"}})
	if err != nil {
		t.Fatal(err)
	}
	var out appImportExitOutput
	decodeStructured(t, res, &out)
	if method != http.MethodGet || path != "/api/v1/migration/apps/sessions/appimp-1/exit-report" {
		t.Errorf("request = %s %s", method, path)
	}
	if out.Counts["manual"] != 1 || len(out.Open) != 1 || out.Open[0] != "manual volume web/uploads" {
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

func TestGetMigrationJobStatusOnlyReads(t *testing.T) {
	var calls []string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/retire") {
			_, _ = w.Write([]byte(`{"safe_to_delete":false,"gates":[{"id":"volumes","status":"fail","detail":"1 of 1 volume(s) are not copied"},{"id":"staged","status":"pass","detail":"ok"}],"fallback":[]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.MigrationJob{ID: "appimp-1", Platform: "dokploy", Phase: "attention", Total: 2, Done: 1, Failed: 1,
			Next: []string{"retry"}, Items: []apiclient.MigrationJobItem{{Name: "api", State: "stage-failed", Reason: "pull denied", Retryable: true}, {Name: "web", State: "verified"}}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_migration_job_status", Arguments: map[string]any{"id": "appimp-1"}})
	if err != nil {
		t.Fatal(err)
	}
	var out migrationJobOutput
	decodeStructured(t, res, &out)
	for _, c := range calls {
		if !strings.HasPrefix(c, "GET ") {
			t.Errorf("a read tool made a %s", c)
		}
	}
	if out.SafeToDelete || len(out.Failed) != 1 || !strings.HasPrefix(out.Failed[0], "api (stage-failed)") || len(out.OpenGates) != 1 || !strings.HasPrefix(out.OpenGates[0], "volumes") {
		t.Errorf("out = %+v", out)
	}
}
