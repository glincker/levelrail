package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestQueryLogsCapsOutput(t *testing.T) {
	t.Setenv(apiclient.EnvLogMaxBytes, "1500")
	now := time.Now().UTC()
	var gotQuery map[string]string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = map[string]string{}
		for k := range r.URL.Query() {
			gotQuery[k] = r.URL.Query().Get(k)
		}
		entries := make([]map[string]any, 100)
		for i := range entries {
			entries[i] = map[string]any{"timestamp": now.Add(time.Duration(i) * time.Second).Format(time.RFC3339), "message": fmt.Sprintf("line %03d %s", i, strings.Repeat("y", 80)), "level": "error"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 1812, "entries": entries})
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "query_logs", Arguments: map[string]any{"app": "web", "level": "error", "since": "30m", "text": "timeout", "max_lines": 100}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %s", err, toolResultText(res))
	}
	var ex apiclient.LogExcerpt
	decodeStructured(t, res, &ex)
	if ex.Matched != 1812 || ex.Shown == 0 || ex.Shown >= 100 {
		t.Errorf("matched=%d shown=%d", ex.Matched, ex.Shown)
	}
	if !strings.Contains(ex.Notice, "of 1,812 matching lines") {
		t.Errorf("notice = %q", ex.Notice)
	}
	if raw, _ := json.Marshal(ex); len(raw) > 1500 {
		t.Errorf("excerpt is %d bytes, cap 1500", len(raw))
	}
	if !strings.Contains(ex.Lines[len(ex.Lines)-1], "line 099") {
		t.Errorf("newest line should be kept: %q", ex.Lines[len(ex.Lines)-1])
	}
	if gotQuery["level"] != "error" || gotQuery["q"] != "timeout" || gotQuery["limit"] != "100" {
		t.Errorf("api query = %v", gotQuery)
	}
}

func TestQueryLogsRejectsBadWindow(t *testing.T) {
	session := newTestSession(t, func(http.ResponseWriter, *http.Request) { t.Error("no API call expected") })
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "query_logs", Arguments: map[string]any{"app": "web", "since": "nonsense"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(toolResultText(res), "since") {
		t.Errorf("result = %s", toolResultText(res))
	}
}
