package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetAppRequests(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/requests" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.AppRequestsResource{
			App:     "web",
			Summary: apiclient.RequestSummaryResource{HasTraffic: true, Requests: 10, P95Ms: 120},
		})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_app_requests",
		Arguments: map[string]any{"name": "web", "since": "30m"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got apiclient.AppRequestsResource
	decodeStructured(t, result, &got)
	if got.App != "web" || got.Summary.P95Ms != 120 {
		t.Errorf("got %+v", got)
	}
}

func TestGetAppRequests_Surface403(t *testing.T) {
	assertToolsSurface403(t, []toolCase{{"get_app_requests", map[string]any{"name": "web"}}})
}
