package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLogArchiveTools(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/storage/destinations":
			_ = json.NewEncoder(w).Encode([]apiclient.StorageDestination{{ID: "bkt_1", Bucket: "logs"}})
		case "/api/v1/storage/destinations/bkt_1/test":
			_ = json.NewEncoder(w).Encode(apiclient.StorageProbeResult{OK: true})
		case "/api/v1/log-archive/policy":
			_ = json.NewEncoder(w).Encode(apiclient.LogArchivePolicy{ID: "lap_1", AppName: "web"})
		case "/api/v1/log-archive/dump":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.LogArchiveRun{ID: "lar_1", Status: "running"})
		default:
			_ = json.NewEncoder(w).Encode([]string{})
		}
	})
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"list_storage_destinations", map[string]any{}},
		{"test_storage_destination", map[string]any{"id": "bkt_1"}},
		{"set_log_archive_policy", map[string]any{"target_id": "bkt_1", "app_name": "web"}},
		{"start_log_archive_dump", map[string]any{"target_id": "bkt_1", "from": "2026-09-24T00:00:00Z", "to": "2026-09-24T01:00:00Z"}},
		{"list_log_archive_policies", map[string]any{}},
		{"list_log_archive_runs", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil || res.IsError {
				t.Fatalf("CallTool(%s) err=%v isError=%v", tt.tool, err, res != nil && res.IsError)
			}
		})
	}
}

func TestLogArchiveToolsSurface403(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_storage_destinations", Arguments: map[string]any{}})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatal("expected an error result for a 403")
	}
}
