package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestRun_Plugins_RunCommands(t *testing.T) {
	experimental.Set(experimental.Plugins)
	restoreAllExperimental(t)
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/plugins/example":
			_ = json.NewEncoder(w).Encode(apiclient.Plugin{Name: "example", Commands: []apiclient.PluginCommand{
				{ID: "deploys", Panel: "deploys"}, {ID: "refresh", Panel: "deploys", Action: "refresh"},
			}})
		case strings.HasSuffix(r.URL.Path, "/data"):
			_ = json.NewEncoder(w).Encode(apiclient.PluginPanelView{Widgets: []apiclient.PluginWidgetView{
				{Type: "table", Title: "Latest", Columns: []apiclient.PluginPanelColumn{{Key: "sha", Label: "Commit"}}, Rows: [][]string{{"abc123"}}},
			}})
		default:
			_ = json.NewEncoder(w).Encode(apiclient.PluginActionResult{OK: true, Message: "queued"})
		}
	}))
	defer srv.Close()
	api := []string{"--api-url", srv.URL, "--token", "t"}

	out, _ := runCLIExpectOK(t, append([]string{"plugins", "run", "example", "deploys", "--app", "web"}, api...))
	if !strings.Contains(out, "COMMIT") || !strings.Contains(out, "abc123") {
		t.Errorf("data command output = %q", out)
	}
	out, _ = runCLIExpectOK(t, append([]string{"plugins", "run", "example", "refresh"}, api...))
	if !strings.Contains(out, "ok: queued") {
		t.Errorf("action command output = %q", out)
	}
	want := []string{
		"GET /api/v1/plugins/example",
		"GET /api/v1/plugins/example/panels/deploys/data?app_id=web",
		"GET /api/v1/plugins/example",
		"POST /api/v1/plugins/example/panels/deploys/actions/refresh",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Errorf("calls = %v, want %v", paths, want)
	}
}
