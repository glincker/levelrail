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

func TestLoadBalancerHistoryTools(t *testing.T) {
	var requests []string
	var putBody map[string]string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/history"):
			_, _ = w.Write([]byte(`{"upstreams":[{"id":"web#0","dial":"a:1","admin_state":"active","checks":[],"transitions":[{"at":"2026-01-01T00:00:00Z","from":"healthy","to":"unhealthy","reason":"connection refused"}],"series":{"connections":[],"latency_ms":[],"fails":[]}}]}`))
		case strings.HasSuffix(r.URL.Path, "/check"):
			_, _ = w.Write([]byte(`{"results":[{"id":"web#0","dial":"a:1","ok":true,"status_code":200,"latency_ms":4,"reason":""}]}`))
		default:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_, _ = w.Write([]byte(`{"id":"web#0","dial":"a:1","state":"disabled","admin_state":"disabled"}`))
		}
	})
	call := func(name string, args map[string]any, out any) {
		t.Helper()
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("CallTool(%s) error = %v", name, err)
		}
		decodeStructured(t, res, out)
	}

	var hist apiclient.LoadBalancerHistory
	call("get_load_balancer_history", map[string]any{"name": "web", "limit": 10}, &hist)
	if len(hist.Upstreams) != 1 || hist.Upstreams[0].Transitions[0].Reason != "connection refused" {
		t.Errorf("history = %+v", hist)
	}
	var chk apiclient.LoadBalancerCheck
	call("check_load_balancer", map[string]any{"name": "web"}, &chk)
	if len(chk.Results) != 1 || !chk.Results[0].OK {
		t.Errorf("check = %+v", chk)
	}
	var up apiclient.LoadBalancerUpstreamStatus
	call("set_load_balancer_upstream_state", map[string]any{"name": "web", "upstream_id": "web#0", "state": "disabled"}, &up)
	if up.AdminState != "disabled" || putBody["admin_state"] != "disabled" {
		t.Errorf("set state = %+v body %v", up, putBody)
	}
	if len(requests) != 3 || requests[0] != "GET /api/v1/apps/web/loadbalancer/history?limit=10" || requests[1] != "POST /api/v1/apps/web/loadbalancer/check?" {
		t.Errorf("requests = %v", requests)
	}
	if !strings.HasPrefix(requests[2], "PUT /api/v1/apps/web/loadbalancer/upstreams/web%230") {
		t.Errorf("upstream request = %q", requests[2])
	}
}

func TestLoadBalancerHistoryTools_Classes(t *testing.T) {
	tests := []struct {
		tool      string
		class     Class
		untrusted bool
	}{
		{"get_load_balancer_history", ClassRead, true},
		{"check_load_balancer", ClassMutate, false},
		{"set_load_balancer_upstream_state", ClassMutate, false},
	}
	for _, tt := range tests {
		m, ok := toolTable[tt.tool]
		if !ok || m.Class != tt.class || m.Untrusted() != tt.untrusted {
			t.Errorf("%s meta = %+v ok=%v, want class %v untrusted %v", tt.tool, m, ok, tt.class, tt.untrusted)
		}
	}
}
