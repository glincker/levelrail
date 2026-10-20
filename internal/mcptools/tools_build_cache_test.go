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

func TestBuildCacheTools(t *testing.T) {
	var gotPut apiclient.BuildCacheRequest
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&gotPut)
			_ = json.NewEncoder(w).Encode(apiclient.BuildCacheSetting{AppName: gotPut.AppName, TargetID: gotPut.TargetID, Mode: "max", Enabled: true})
		case r.URL.Path == "/api/v1/build-cache/stats":
			_ = json.NewEncoder(w).Encode(apiclient.BuildCacheStats{Objects: 3, Bytes: 12})
		default:
			_ = json.NewEncoder(w).Encode([]apiclient.BuildCacheSetting{{AppName: "web", TargetID: "bkt_1", Mode: "max", Enabled: true}, {AppName: "api", TargetID: "bkt_1"}})
		}
	})
	call := func(tool string, args map[string]any) string {
		t.Helper()
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("CallTool(%s) err=%v isError=%v", tool, err, res != nil && res.IsError)
		}
		b, _ := json.Marshal(res.StructuredContent)
		return string(b)
	}

	out := call("get_build_cache", map[string]any{"app_name": "web", "stats": true})
	if !strings.Contains(out, `"web"`) || strings.Contains(out, `"api"`) || !strings.Contains(out, `"objects":3`) {
		t.Fatalf("get_build_cache = %s", out)
	}
	if out := call("get_build_cache", map[string]any{}); !strings.Contains(out, `"api"`) {
		t.Fatalf("unfiltered get_build_cache = %s", out)
	}
	call("set_build_cache", map[string]any{"app_name": "web", "target_id": "bkt_1", "mode": "min"})
	if gotPut.AppName != "web" || gotPut.TargetID != "bkt_1" || gotPut.Mode != "min" {
		t.Fatalf("PUT body = %+v", gotPut)
	}
}
