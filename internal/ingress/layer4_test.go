package ingress

import (
	"encoding/json"
	"testing"
)

func TestBuildRoutesConfig_Streams_JSONShape(t *testing.T) {
	t.Run("no streams: no layer4 app at all", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "ingress", ListenAddr: ":443"})
		if err != nil {
			t.Fatalf("BuildRoutesConfig() error: %v", err)
		}
		if cfg.Apps.Layer4 != nil {
			t.Errorf("Apps.Layer4 = %+v, want nil with zero streams", cfg.Apps.Layer4)
		}
	})

	t.Run("two streams: one dedicated layer4 server each, unconditional proxy handler", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress",
			ListenAddr: ":443",
			Streams: []StreamRoute{
				{ListenAddr: ":15432", BackendDial: "127.0.0.1:30001"},
				{ListenAddr: ":16379", BackendDial: "127.0.0.1:30002"},
			},
		})
		if err != nil {
			t.Fatalf("BuildRoutesConfig() error: %v", err)
		}

		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("json.Marshal() error: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("json.Unmarshal() error: %v", err)
		}

		apps := decoded["apps"].(map[string]any)
		layer4 := apps["layer4"].(map[string]any)
		servers := layer4["servers"].(map[string]any)
		if len(servers) != 2 {
			t.Fatalf("len(servers) = %d, want 2: %+v", len(servers), servers)
		}

		var sawListen15432, sawListen16379 bool
		for _, raw := range servers {
			srv := raw.(map[string]any)
			listen := srv["listen"].([]any)
			if len(listen) != 1 {
				t.Fatalf("listen = %v, want exactly one address", listen)
			}
			routes := srv["routes"].([]any)
			if len(routes) != 1 {
				t.Fatalf("routes = %v, want exactly one route", routes)
			}
			route := routes[0].(map[string]any)
			if _, hasMatch := route["match"]; hasMatch {
				t.Errorf("route.match present, want omitted (match-everything on this dedicated port)")
			}
			handle := route["handle"].([]any)
			if len(handle) != 1 {
				t.Fatalf("handle = %v, want exactly one handler", handle)
			}
			h := handle[0].(map[string]any)
			if h["handler"] != "proxy" {
				t.Errorf("handler = %v, want \"proxy\"", h["handler"])
			}
			upstreams := h["upstreams"].([]any)
			if len(upstreams) != 1 {
				t.Fatalf("upstreams = %v, want exactly one", upstreams)
			}
			dial := upstreams[0].(map[string]any)["dial"].([]any)
			switch listen[0] {
			case ":15432":
				sawListen15432 = true
				if len(dial) != 1 || dial[0] != "127.0.0.1:30001" {
					t.Errorf("dial = %v, want [127.0.0.1:30001]", dial)
				}
			case ":16379":
				sawListen16379 = true
				if len(dial) != 1 || dial[0] != "127.0.0.1:30002" {
					t.Errorf("dial = %v, want [127.0.0.1:30002]", dial)
				}
			default:
				t.Errorf("unexpected listen address %v", listen[0])
			}
		}
		if !sawListen15432 || !sawListen16379 {
			t.Errorf("missing an expected listener: sawListen15432=%v sawListen16379=%v", sawListen15432, sawListen16379)
		}

		// A stream's own listener never shares a server with the HTTP
		// app: no HTTP routes were configured in this test, so
		// apps.http.servers.ingress still exists (ListenAddr is always
		// bound) but carries no routes.
		http := apps["http"].(map[string]any)
		httpServers := http["servers"].(map[string]any)
		ingressSrv := httpServers["ingress"].(map[string]any)
		if _, hasRoutes := ingressSrv["routes"]; hasRoutes {
			t.Errorf("http server routes present, want omitted: no HTTP routes were configured in this test")
		}
	})

	t.Run("stream with no listen address is rejected", func(t *testing.T) {
		_, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress", ListenAddr: ":443",
			Streams: []StreamRoute{{BackendDial: "127.0.0.1:30001"}},
		})
		if err == nil {
			t.Fatal("BuildRoutesConfig() error = nil, want an error for a stream with no listen address")
		}
	})

	t.Run("stream with no backend dial is rejected", func(t *testing.T) {
		_, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress", ListenAddr: ":443",
			Streams: []StreamRoute{{ListenAddr: ":15432"}},
		})
		if err == nil {
			t.Fatal("BuildRoutesConfig() error = nil, want an error for a stream with no backend dial address")
		}
	})
}
