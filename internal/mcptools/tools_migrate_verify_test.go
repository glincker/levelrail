package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrateverify"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetMigrationVerifyReportIsReadOnlyGet(t *testing.T) {
	var method, path string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.MigrationVerifyReport{Report: migrateverify.Report{Verdict: migrateverify.VerdictFail,
			Counts: migrateverify.Counts{Blocking: 1},
			Differences: []migrateverify.Difference{{Category: migrateverify.CatEnv, Severity: migrateverify.Blocking, App: "web", Subject: "DB_URL",
				Detail: "variable name missing here", Fix: "set DB_URL"}}}})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_migration_verify_report"})
	if err != nil {
		t.Fatal(err)
	}
	var out migrationVerifyOutput
	decodeStructured(t, res, &out)
	if method != http.MethodGet || path != "/api/v1/migrations/verify/latest" {
		t.Errorf("request = %s %s", method, path)
	}
	if !strings.HasPrefix(out.Verdict, "fail (1 blocking") || len(out.Differences) != 1 || !strings.Contains(out.Differences[0], "web/DB_URL") || !strings.Contains(out.Differences[0], "Fix: set DB_URL") {
		t.Errorf("out = %+v", out)
	}
	if meta, ok := toolTable["get_migration_verify_report"]; !ok || meta.Class != ClassRead {
		t.Errorf("get_migration_verify_report must be a classified read tool: %+v", meta)
	}
}
