package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildProxyConfig_Validation covers the pure-logic error paths: no
// Caddy instance is started for any of these, this is just Go struct
// construction and validation.
func TestBuildProxyConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		opts    ProxyOptions
		wantErr string // substring expected in the error, "" means no error
	}{
		{
			name:    "missing server name",
			opts:    ProxyOptions{ListenAddr: ":8080", BackendDial: "127.0.0.1:9090"},
			wantErr: "server name is required",
		},
		{
			name:    "missing listen address",
			opts:    ProxyOptions{ServerName: "web", BackendDial: "127.0.0.1:9090"},
			wantErr: "listen address is required",
		},
		{
			name:    "missing backend dial",
			opts:    ProxyOptions{ServerName: "web", ListenAddr: ":8080"},
			wantErr: "backend dial address is required",
		},
		{
			name: "TLS without hosts",
			opts: ProxyOptions{
				ServerName: "web", ListenAddr: ":8443", BackendDial: "127.0.0.1:9090",
				TLS: true,
			},
			wantErr: "TLS requires at least one host",
		},
		{
			name: "valid plain HTTP, no hosts",
			opts: ProxyOptions{
				ServerName: "web", ListenAddr: ":8080", BackendDial: "127.0.0.1:9090",
			},
			wantErr: "",
		},
		{
			name: "valid TLS with hosts",
			opts: ProxyOptions{
				ServerName: "web", ListenAddr: ":8443", BackendDial: "127.0.0.1:9090",
				Hosts: []string{"app.example.internal"}, TLS: true,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildProxyConfig(tt.opts)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("BuildProxyConfig() unexpected error: %v", err)
				}
				if cfg == nil {
					t.Fatalf("BuildProxyConfig() returned nil config with no error")
				}
				return
			}

			if err == nil {
				t.Fatalf("BuildProxyConfig() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("BuildProxyConfig() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
			if cfg != nil {
				t.Errorf("BuildProxyConfig() returned non-nil config alongside an error")
			}
		})
	}
}

// TestBuildProxyConfig_JSONShape asserts the marshaled JSON matches
// Caddy's actual config schema at the specific field paths this spike
// relies on (apps.http.servers[name].listen/routes, apps.tls.automation).
// This is what catches a typo in a json tag before it ever reaches a live
// Caddy instance and fails opaquely inside module Provision.
func TestBuildProxyConfig_JSONShape(t *testing.T) {
	t.Run("plain HTTP, no TLS app present", func(t *testing.T) {
		cfg, err := BuildProxyConfig(ProxyOptions{
			ServerName:  "web",
			ListenAddr:  ":8080",
			BackendDial: "127.0.0.1:9090",
			Hosts:       []string{"app.example.internal"},
		})
		if err != nil {
			t.Fatalf("BuildProxyConfig() error: %v", err)
		}

		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("json.Marshal() error: %v", err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("json.Unmarshal() error: %v", err)
		}

		apps, ok := decoded["apps"].(map[string]any)
		if !ok {
			t.Fatalf("decoded config has no apps object: %v", decoded)
		}
		if _, hasTLS := apps["tls"]; hasTLS {
			t.Errorf("apps.tls present for a non-TLS config, want it omitted entirely")
		}

		http, ok := apps["http"].(map[string]any)
		if !ok {
			t.Fatalf("apps.http missing or wrong type: %v", apps)
		}
		servers, ok := http["servers"].(map[string]any)
		if !ok {
			t.Fatalf("apps.http.servers missing or wrong type: %v", http)
		}
		web, ok := servers["web"].(map[string]any)
		if !ok {
			t.Fatalf("apps.http.servers.web missing or wrong type: %v", servers)
		}

		listen, ok := web["listen"].([]any)
		if !ok || len(listen) != 1 || listen[0] != ":8080" {
			t.Errorf("apps.http.servers.web.listen = %v, want [\":8080\"]", web["listen"])
		}

		routes, ok := web["routes"].([]any)
		if !ok || len(routes) != 1 {
			t.Fatalf("apps.http.servers.web.routes = %v, want exactly one route", web["routes"])
		}
		route, ok := routes[0].(map[string]any)
		if !ok {
			t.Fatalf("route 0 is not an object: %v", routes[0])
		}
		handle, ok := route["handle"].([]any)
		if !ok || len(handle) != 1 {
			t.Fatalf("route.handle = %v, want exactly one handler", route["handle"])
		}
		handler, ok := handle[0].(map[string]any)
		if !ok {
			t.Fatalf("handler 0 is not an object: %v", handle[0])
		}
		if handler["handler"] != "reverse_proxy" {
			t.Errorf("handler.handler = %v, want %q", handler["handler"], "reverse_proxy")
		}
		upstreams, ok := handler["upstreams"].([]any)
		if !ok || len(upstreams) != 1 {
			t.Fatalf("handler.upstreams = %v, want exactly one upstream", handler["upstreams"])
		}
		upstream, ok := upstreams[0].(map[string]any)
		if !ok || upstream["dial"] != "127.0.0.1:9090" {
			t.Errorf("upstream = %v, want dial 127.0.0.1:9090", upstreams[0])
		}
	})

	t.Run("TLS enabled: internal issuer scoped to hosts, redirects disabled", func(t *testing.T) {
		cfg, err := BuildProxyConfig(ProxyOptions{
			ServerName:  "web",
			ListenAddr:  ":8443",
			BackendDial: "127.0.0.1:9090",
			Hosts:       []string{"app.example.internal", "api.example.internal"},
			TLS:         true,
		})
		if err != nil {
			t.Fatalf("BuildProxyConfig() error: %v", err)
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
		tlsApp, ok := apps["tls"].(map[string]any)
		if !ok {
			t.Fatalf("apps.tls missing for a TLS config: %v", apps)
		}
		automation := tlsApp["automation"].(map[string]any)
		policies := automation["policies"].([]any)
		if len(policies) != 1 {
			t.Fatalf("apps.tls.automation.policies = %v, want exactly one policy", policies)
		}
		policy := policies[0].(map[string]any)

		subjects, ok := policy["subjects"].([]any)
		if !ok || len(subjects) != 2 {
			t.Fatalf("policy.subjects = %v, want the two configured hosts", policy["subjects"])
		}

		issuers, ok := policy["issuers"].([]any)
		if !ok || len(issuers) != 1 {
			t.Fatalf("policy.issuers = %v, want exactly one issuer", policy["issuers"])
		}
		issuer := issuers[0].(map[string]any)
		if issuer["module"] != "internal" {
			t.Errorf("issuer.module = %v, want %q", issuer["module"], "internal")
		}

		http := apps["http"].(map[string]any)
		web := http["servers"].(map[string]any)["web"].(map[string]any)
		autoHTTPS, ok := web["automatic_https"].(map[string]any)
		if !ok {
			t.Fatalf("apps.http.servers.web.automatic_https missing: %v", web)
		}
		if autoHTTPS["disable_redirects"] != true {
			t.Errorf("automatic_https.disable_redirects = %v, want true", autoHTTPS["disable_redirects"])
		}

		route := web["routes"].([]any)[0].(map[string]any)
		match, ok := route["match"].([]any)
		if !ok || len(match) != 1 {
			t.Fatalf("route.match = %v, want exactly one matcher set", route["match"])
		}
		matcher := match[0].(map[string]any)
		host, ok := matcher["host"].([]any)
		if !ok || len(host) != 2 {
			t.Errorf("matcher.host = %v, want the two configured hosts", matcher["host"])
		}
	})

	t.Run("admin listen empty by default (Caddy's own default applies), set when requested, persist always disabled", func(t *testing.T) {
		cfgDefault, err := BuildProxyConfig(ProxyOptions{
			ServerName: "web", ListenAddr: ":8080", BackendDial: "127.0.0.1:9090",
		})
		if err != nil {
			t.Fatalf("BuildProxyConfig() error: %v", err)
		}
		if cfgDefault.Admin == nil {
			t.Fatalf("Admin = nil, want non-nil (Config.Persist is always set)")
		}
		if cfgDefault.Admin.Listen != "" {
			t.Errorf("Admin.Listen = %q, want empty so Caddy's own default (localhost:2019) applies", cfgDefault.Admin.Listen)
		}
		if cfgDefault.Admin.Config == nil || cfgDefault.Admin.Config.Persist == nil || *cfgDefault.Admin.Config.Persist != false {
			t.Errorf("Admin.Config.Persist = %+v, want a pointer to false", cfgDefault.Admin.Config)
		}

		cfgCustom, err := BuildProxyConfig(ProxyOptions{
			ServerName: "web", ListenAddr: ":8080", BackendDial: "127.0.0.1:9090",
			AdminListen: "localhost:2021",
		})
		if err != nil {
			t.Fatalf("BuildProxyConfig() error: %v", err)
		}
		if cfgCustom.Admin == nil || cfgCustom.Admin.Listen != "localhost:2021" {
			t.Errorf("Admin = %+v, want Listen=localhost:2021", cfgCustom.Admin)
		}
	})
}

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
}
