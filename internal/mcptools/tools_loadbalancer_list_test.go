package mcptools

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListLoadBalancersTool(t *testing.T) {
	var gotQuery string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"app":"web","service":"web","algorithm":"least_conn","state":"degraded","upstreams_total":2,"upstreams_healthy":1}],"total":1,"limit":100,"offset":0}`))
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_load_balancers", Arguments: map[string]any{"state": "degraded", "search": "we"}})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
	var got apiclient.LoadBalancerList
	decodeStructured(t, res, &got)
	if len(got.Items) != 1 || got.Items[0].State != "degraded" || got.Items[0].UpstreamsHealthy != 1 {
		t.Errorf("list = %+v", got)
	}
	if gotQuery != "q=we&state=degraded" {
		t.Errorf("query = %q, want q=we&state=degraded", gotQuery)
	}
}
