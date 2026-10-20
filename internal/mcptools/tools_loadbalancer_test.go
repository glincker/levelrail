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

func TestLoadBalancerTools(t *testing.T) {
	var lastPut apiclient.LoadBalancerConfig
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&lastPut)
			_ = json.NewEncoder(w).Encode(apiclient.LoadBalancerResource{AppName: "web", Configured: true, Config: &lastPut})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = w.Write([]byte(`{"service":"web","algorithm":"least_conn","ready":true,"reason":"Balancing","upstreams":[{"id":"web#0","dial":"a:1","state":"healthy"}]}`))
		case strings.HasSuffix(r.URL.Path, "/export"):
			_ = json.NewEncoder(w).Encode(apiclient.LoadBalancerArtifact{Format: r.URL.Query().Get("format"), Filename: "x.tf", Body: "hcl"})
		default:
			_ = json.NewEncoder(w).Encode(apiclient.LoadBalancerResource{AppName: "web"})
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

	var got apiclient.LoadBalancerResource
	call("get_app_load_balancer", map[string]any{"name": "web"}, &got)
	if got.AppName != "web" || got.Configured {
		t.Errorf("get = %+v", got)
	}

	call("set_app_load_balancer", map[string]any{"name": "web", "config": map[string]any{"algorithm": "least_conn", "drain_timeout": "10s"}}, &got)
	if !got.Configured || lastPut.Algorithm != "least_conn" || lastPut.DrainTimeout != "10s" {
		t.Errorf("set = %+v sent = %+v", got, lastPut)
	}

	var st apiclient.LoadBalancerStatus
	call("get_app_load_balancer_status", map[string]any{"name": "web"}, &st)
	if len(st.Upstreams) != 1 || st.Upstreams[0].State != "healthy" {
		t.Errorf("status = %+v", st)
	}

	var art apiclient.LoadBalancerArtifact
	call("export_app_load_balancer", map[string]any{"name": "web", "format": "terraform"}, &art)
	if art.Format != "terraform" || art.Body != "hcl" {
		t.Errorf("export = %+v", art)
	}

	var cleared struct{ Cleared bool }
	call("clear_app_load_balancer", map[string]any{"name": "web"}, &cleared)
	if !cleared.Cleared {
		t.Error("clear = false")
	}
}

func TestLoadBalancerTools_Surface403(t *testing.T) {
	for _, tool := range []string{"get_app_load_balancer", "get_app_load_balancer_status", "clear_app_load_balancer"} {
		t.Run(tool, func(t *testing.T) {
			session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden"}`))
			})
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"name": "web"}})
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError || !strings.Contains(toolResultText(res), "forbidden") {
				t.Errorf("result = %+v", res)
			}
		})
	}
}
