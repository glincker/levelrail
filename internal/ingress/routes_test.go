package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildRoutesConfig_Validation covers the pure-logic error paths for
// the multi-service builder the ingress reconcile controller
// (internal/reconcile/ingress, TASKS.md 1.6) uses to assemble one config
// from every currently routable service.
func TestBuildRoutesConfig_Validation(t *testing.T) {
	validRoute := ProxyRoute{Hosts: []string{"app.example.internal"}, BackendDial: "127.0.0.1:9090"}

	tests := []struct {
		name    string
		opts    RoutesOptions
		wantErr string
	}{
		{
			name:    "missing server name",
			opts:    RoutesOptions{ListenAddr: ":8443", Routes: []ProxyRoute{validRoute}},
			wantErr: "server name is required",
		},
		{
			name:    "missing listen address",
			opts:    RoutesOptions{ServerName: "ingress", Routes: []ProxyRoute{validRoute}},
			wantErr: "listen address is required",
		},
		{
			name: "route with no hosts",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":8443",
				Routes: []ProxyRoute{{BackendDial: "127.0.0.1:9090"}},
			},
			wantErr: "route 0 has no hosts",
		},
		{
			name: "route with no backend dial",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":8443",
				Routes: []ProxyRoute{{Hosts: []string{"app.example.internal"}}},
			},
			wantErr: "route 0 has no backend dial address",
		},
		{
			name:    "empty routes is valid, not an error",
			opts:    RoutesOptions{ServerName: "ingress", ListenAddr: ":8443"},
			wantErr: "",
		},
		{
			name:    "one valid route",
			opts:    RoutesOptions{ServerName: "ingress", ListenAddr: ":8443", Routes: []ProxyRoute{validRoute}},
			wantErr: "",
		},
		{
			name: "static route with no hosts",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":8443",
				StaticRoutes: []StaticRoute{{RootDir: "/data/static/docs"}},
			},
			wantErr: "static route 0 has no hosts",
		},
		{
			name: "static route with no root directory",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":8443",
				StaticRoutes: []StaticRoute{{Hosts: []string{"docs.example.internal"}}},
			},
			wantErr: "static route 0 has no root directory",
		},
		{
			name: "one valid static route, no proxy routes at all",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":8443",
				StaticRoutes: []StaticRoute{{Hosts: []string{"docs.example.internal"}, RootDir: "/data/static/docs"}},
			},
			wantErr: "",
		},
		{
			name: "one proxy route and one static route together",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":8443",
				Routes:       []ProxyRoute{validRoute},
				StaticRoutes: []StaticRoute{{Hosts: []string{"docs.example.internal"}, RootDir: "/data/static/docs"}},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildRoutesConfig(tt.opts)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("BuildRoutesConfig() unexpected error: %v", err)
				}
				if cfg == nil {
					t.Fatalf("BuildRoutesConfig() returned nil config with no error")
				}
				return
			}

			if err == nil {
				t.Fatalf("BuildRoutesConfig() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("BuildRoutesConfig() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
			if cfg != nil {
				t.Errorf("BuildRoutesConfig() returned non-nil config alongside an error")
			}
		})
	}
}

// TestBuildRoutesConfig_JSONShape asserts the marshaled JSON shape at the
// specific field paths BuildRoutesConfig relies on: multiple routes
// inside one server, each with its own host matcher and upstream, and a
// single TLS automation policy whose subjects span every route's hosts.
func TestBuildRoutesConfig_JSONShape(t *testing.T) {
	t.Run("empty routes: listener present, no routes, no TLS app", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress", ListenAddr: ":443", TLS: true,
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
		if _, hasTLS := apps["tls"]; hasTLS {
			t.Errorf("apps.tls present with zero routes, want it omitted: nothing to issue a certificate for")
		}
		http := apps["http"].(map[string]any)
		servers := http["servers"].(map[string]any)
		ingressSrv := servers["ingress"].(map[string]any)
		listen, ok := ingressSrv["listen"].([]any)
		if !ok || len(listen) != 1 || listen[0] != ":443" {
			t.Errorf("listen = %v, want [\":443\"]", ingressSrv["listen"])
		}
		if _, hasRoutes := ingressSrv["routes"]; hasRoutes {
			t.Errorf("routes present with zero configured routes, want it omitted (empty slice)")
		}
	})

	t.Run("two routes, distinct hosts and backends, one shared TLS policy", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress",
			ListenAddr: ":443",
			TLS:        true,
			Routes: []ProxyRoute{
				{Hosts: []string{"a.example.internal"}, BackendDial: "127.0.0.1:9001"},
				{Hosts: []string{"b.example.internal"}, BackendDial: "127.0.0.1:9002"},
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
		http := apps["http"].(map[string]any)
		servers := http["servers"].(map[string]any)
		ingressSrv := servers["ingress"].(map[string]any)
		routes, ok := ingressSrv["routes"].([]any)
		if !ok || len(routes) != 2 {
			t.Fatalf("routes = %v, want exactly two", ingressSrv["routes"])
		}

		gotDials := map[string]string{}
		for _, r := range routes {
			route := r.(map[string]any)
			match := route["match"].([]any)[0].(map[string]any)
			host := match["host"].([]any)[0].(string)
			handler := route["handle"].([]any)[0].(map[string]any)
			upstream := handler["upstreams"].([]any)[0].(map[string]any)
			gotDials[host] = upstream["dial"].(string)
		}
		want := map[string]string{"a.example.internal": "127.0.0.1:9001", "b.example.internal": "127.0.0.1:9002"}
		if len(gotDials) != len(want) {
			t.Fatalf("gotDials = %v, want %v", gotDials, want)
		}
		for host, dial := range want {
			if gotDials[host] != dial {
				t.Errorf("route for host %q dials %q, want %q", host, gotDials[host], dial)
			}
		}

		tlsApp := apps["tls"].(map[string]any)
		policies := tlsApp["automation"].(map[string]any)["policies"].([]any)
		if len(policies) != 1 {
			t.Fatalf("policies = %v, want exactly one shared policy", policies)
		}
		subjects := policies[0].(map[string]any)["subjects"].([]any)
		if len(subjects) != 2 {
			t.Errorf("subjects = %v, want both hosts across both routes", subjects)
		}
	})

	t.Run("one proxy route and one static route on the same shared server", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress",
			ListenAddr: ":443",
			TLS:        true,
			Routes: []ProxyRoute{
				{Hosts: []string{"app.example.internal"}, BackendDial: "127.0.0.1:9001"},
			},
			StaticRoutes: []StaticRoute{
				{Hosts: []string{"docs.example.internal"}, RootDir: "/data/static/docs/abc1234"},
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
		http := apps["http"].(map[string]any)
		servers := http["servers"].(map[string]any)
		// Both routes live under the very same named server: a static
		// site is not a second Caddy config document or a second
		// listener, it's another Route on the shared one (see
		// StaticRoute's own doc comment).
		if len(servers) != 1 {
			t.Fatalf("servers = %v, want exactly one shared server carrying both routes", servers)
		}
		ingressSrv := servers["ingress"].(map[string]any)
		routes, ok := ingressSrv["routes"].([]any)
		if !ok || len(routes) != 2 {
			t.Fatalf("routes = %v, want exactly two (one proxy, one static)", ingressSrv["routes"])
		}

		var sawFileServer, sawReverseProxy bool
		for _, r := range routes {
			route := r.(map[string]any)
			match := route["match"].([]any)[0].(map[string]any)
			host := match["host"].([]any)[0].(string)
			handler := route["handle"].([]any)[0].(map[string]any)

			switch handler["handler"] {
			case "file_server":
				sawFileServer = true
				if host != "docs.example.internal" {
					t.Errorf("file_server route host = %q, want docs.example.internal", host)
				}
				if handler["root"] != "/data/static/docs/abc1234" {
					t.Errorf("file_server root = %v, want /data/static/docs/abc1234", handler["root"])
				}
				if _, hasUpstreams := handler["upstreams"]; hasUpstreams {
					t.Errorf("file_server handler has an upstreams field, want none: it has no backend to dial")
				}
			case "reverse_proxy":
				sawReverseProxy = true
				if host != "app.example.internal" {
					t.Errorf("reverse_proxy route host = %q, want app.example.internal", host)
				}
			default:
				t.Errorf("unexpected handler %v", handler["handler"])
			}
		}
		if !sawFileServer || !sawReverseProxy {
			t.Errorf("routes = %v, want one file_server route and one reverse_proxy route", routes)
		}

		// TLS automation covers both routes' hosts, one shared policy:
		// a static site gets exactly the same automatic-HTTPS treatment
		// a container service does, no special case.
		tlsApp := apps["tls"].(map[string]any)
		policies := tlsApp["automation"].(map[string]any)["policies"].([]any)
		if len(policies) != 1 {
			t.Fatalf("policies = %v, want exactly one shared policy", policies)
		}
		subjects := policies[0].(map[string]any)["subjects"].([]any)
		if len(subjects) != 2 {
			t.Errorf("subjects = %v, want both hosts across both proxy and static routes", subjects)
		}
	})

	t.Run("static route only, no proxy routes: TLS app still built", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress",
			ListenAddr: ":443",
			TLS:        true,
			StaticRoutes: []StaticRoute{
				{Hosts: []string{"docs.example.internal"}, RootDir: "/data/static/docs"},
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
		if _, hasTLS := apps["tls"]; !hasTLS {
			t.Errorf("apps.tls missing for a static-only route set with TLS: true, want automation built from the static route's hosts")
		}
	})

	t.Run("route with BasicAuth gets an authentication handler ahead of reverse_proxy", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "ingress",
			ListenAddr: ":443",
			Routes: []ProxyRoute{
				{
					Hosts:       []string{"protected.example.internal"},
					BackendDial: "127.0.0.1:9001",
					BasicAuth:   &BasicAuthAccount{Username: "operator", Password: "$2a$10$fakehashfakehashfakehashfa"}, //nolint:gosec // fake bcrypt-shaped fixture, not a real credential
				},
				{Hosts: []string{"open.example.internal"}, BackendDial: "127.0.0.1:9002"},
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
		http := apps["http"].(map[string]any)
		servers := http["servers"].(map[string]any)
		ingressSrv := servers["ingress"].(map[string]any)
		routes := ingressSrv["routes"].([]any)
		if len(routes) != 2 {
			t.Fatalf("routes = %v, want exactly two", routes)
		}

		for _, r := range routes {
			route := r.(map[string]any)
			match := route["match"].([]any)[0].(map[string]any)
			host := match["host"].([]any)[0].(string)
			handle := route["handle"].([]any)

			switch host {
			case "protected.example.internal":
				if len(handle) != 2 {
					t.Fatalf("protected route handle = %v, want [authentication, reverse_proxy]", handle)
				}
				authHandler := handle[0].(map[string]any)
				if authHandler["handler"] != "authentication" {
					t.Errorf("handle[0].handler = %v, want authentication (must run before reverse_proxy)", authHandler["handler"])
				}
				providers := authHandler["providers"].(map[string]any)
				httpBasic := providers["http_basic"].(map[string]any)
				accounts := httpBasic["accounts"].([]any)[0].(map[string]any)
				if accounts["username"] != "operator" {
					t.Errorf("accounts[0].username = %v, want operator", accounts["username"])
				}
				if accounts["password"] != "$2a$10$fakehashfakehashfakehashfa" {
					t.Errorf("accounts[0].password = %v, want the bcrypt hash verbatim (never re-derived here)", accounts["password"])
				}
				if hash := httpBasic["hash"].(map[string]any); hash["algorithm"] != "bcrypt" {
					t.Errorf("hash.algorithm = %v, want bcrypt", hash["algorithm"])
				}
				if handle[1].(map[string]any)["handler"] != "reverse_proxy" {
					t.Errorf("handle[1].handler = %v, want reverse_proxy", handle[1].(map[string]any)["handler"])
				}
			case "open.example.internal":
				if len(handle) != 1 || handle[0].(map[string]any)["handler"] != "reverse_proxy" {
					t.Errorf("open route handle = %v, want exactly [reverse_proxy], no authentication handler", handle)
				}
			default:
				t.Errorf("unexpected host %q", host)
			}
		}
	})
}
