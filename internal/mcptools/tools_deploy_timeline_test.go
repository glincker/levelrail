package mcptools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetDeployTimeline(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deployments/dep_1/timeline" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deploy_id":"dep_1","app":"web","verdict":"rolled_back","live":false,
"events":[
{"id":"trigger","at":"2026-10-01T12:00:00Z","kind":"trigger","code":"trigger.git_push","status":"info","detail":"main abc1234"},
{"id":"log:0","at":"2026-10-01T12:00:01Z","kind":"log","code":"log.line","status":"info","detail":"noisy build output"},
{"id":"watch","at":"2026-10-01T12:01:00Z","kind":"watch","code":"watch.crashloop","status":"failed"}],
"phases":[{"phase":"build","duration_ms":1200}],
"impact":{"complete":true,"before":{"has_traffic":true,"rate_per_sec":10,"error_rate_5xx":0.01,"p95_ms":80},"after":{"has_traffic":true,"rate_per_sec":9,"error_rate_5xx":0.2,"p95_ms":400}}}`))
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_deploy_timeline", Arguments: map[string]any{"deploy_id": "dep_1"}})
	if err != nil {
		t.Fatal(err)
	}
	var out deployTimelineOutput
	decodeStructured(t, result, &out)
	if out.Verdict != "rolled_back" || len(out.Events) != 2 || out.LogLines != 1 {
		t.Fatalf("out = %+v", out)
	}
	if out.Events[1].Code != "watch.crashloop" || out.PhaseMS["build"] != 1200 {
		t.Fatalf("out = %+v", out)
	}
	if !strings.Contains(out.Traffic, "20.0% 5xx") {
		t.Fatalf("traffic = %q", out.Traffic)
	}
	for _, e := range out.Events {
		if strings.Contains(e.Detail, "noisy") {
			t.Fatal("build log lines must not be returned")
		}
	}
}
