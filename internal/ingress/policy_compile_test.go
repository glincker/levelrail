package ingress

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

var updatePolicyGolden = flag.Bool("update-policy-golden", false, "rewrite testdata/policy_*.golden.json")

func goldenPolicy() *RoutePolicy {
	limits := trafficpolicy.DefaultLimits()
	limits.ForwardTimeout = 30 * time.Second
	limits.ForwardDialTimeout = 5 * time.Second
	return &RoutePolicy{
		Policy: trafficpolicy.Policy{
			Redirects: &trafficpolicy.Redirects{ForceHTTPS: true, TrailingSlash: trafficpolicy.SlashAdd, LowercaseHost: true},
			Geo:       &trafficpolicy.Geo{Mode: trafficpolicy.GeoAllow, Countries: []string{"US", "CA"}, Action: trafficpolicy.GeoActionBlock, Exempt: []string{"198.51.100.0/24"}},
			Headers: &trafficpolicy.Headers{
				Rules: []trafficpolicy.HeaderRule{
					{Side: trafficpolicy.SideRequest, Op: trafficpolicy.OpSet, Name: "x-env", Value: "prod"},
					{Side: trafficpolicy.SideResponse, Op: trafficpolicy.OpAdd, Name: "X-One", Value: "1"},
					{Side: trafficpolicy.SideResponse, Op: trafficpolicy.OpRemove, Name: "X-Debug"},
				},
				Security:        &trafficpolicy.SecurityPreset{HSTS: true, ContentTypeOptions: true, FrameOptions: "SAMEORIGIN"},
				CORS:            &trafficpolicy.CORSPreset{Origins: []string{"https://ui.example.com"}, Preflight: true},
				HideServer:      true,
				ForwardedPrefix: "/app",
			},
			Cache: &trafficpolicy.Cache{Enabled: true, Rules: []trafficpolicy.CacheRule{{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/assets"}, TTLSeconds: 300}}},
			Forwarders: &trafficpolicy.Forwarders{Rules: []trafficpolicy.Forwarder{
				{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/api/", Methods: []string{"GET", "POST"}}, Action: trafficpolicy.ActionApp, App: "api", StripPrefix: true},
				{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchRegex, Path: `^/v[0-9]+/`}, Action: trafficpolicy.ActionURL, URL: "https://docs.example.org/base"},
				{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchExact, Path: "/old"}, Action: trafficpolicy.ActionRedirect, URL: "https://example.com", RedirectStatus: 308},
				{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/gone"}, Action: trafficpolicy.ActionApp, App: "stopped"},
			}},
		},
		AppDials: map[string]string{"api": "127.0.0.1:9001"},
		Targets: map[string]trafficpolicy.ResolvedTarget{
			"https://docs.example.org/base": {Scheme: "https", Host: "docs.example.org", Port: "443", Addr: netip.MustParseAddr("93.184.216.34"), BasePath: "/base", ServerName: "docs.example.org"},
		},
		TLSReal:   true,
		HTTPSPort: 443,
		GeoActive: true,
		Limits:    limits,
	}
}

func TestPolicyCompile_Golden(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "srv",
		ListenAddr: ":443",
		Routes: []ProxyRoute{{
			Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9000",
			BasicAuth: &BasicAuthAccount{Username: "u", Password: "$2a$10$hash"},
			Policy:    goldenPolicy(),
		}},
		RequestStats: true,
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}
	got, err := json.MarshalIndent(cfg.Apps.HTTP.Servers["srv"].Routes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "policy_route.golden.json")
	if *updatePolicyGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(got, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("read golden (run with -update-policy-golden): %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got)) {
		t.Fatalf("compiled routes differ from %s; rerun with -update-policy-golden and review the diff\n%s", path, got)
	}
}

// handlerNames lists the handler field of each element, unwrapping nothing.
func handlerNames(t *testing.T, handle []any) []string {
	t.Helper()
	var out []string
	for _, h := range handle {
		raw, _ := json.Marshal(h)
		var probe struct {
			Handler string `json:"handler"`
		}
		_ = json.Unmarshal(raw, &probe)
		out = append(out, probe.Handler)
	}
	return out
}

func TestPolicyCompile_Order(t *testing.T) {
	tests := []struct {
		name   string
		route  ProxyRoute
		stats  bool
		expect string
	}{
		{"nil policy unchanged", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1"}, false, "reverse_proxy"},
		{"empty policy unchanged", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1", Policy: &RoutePolicy{}}, false, "reverse_proxy"},
		{"full chain", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1", WAF: &WAFConfig{RateLimitRPS: 5}, BasicAuth: &BasicAuthAccount{Username: "u", Password: "p"}, Policy: goldenPolicy()}, true,
			"request_stats levelrail_redirects levelrail_geo headers headers subroute headers headers headers subroute headers rate_limit authentication levelrail_cache subroute reverse_proxy"},
		{"geo skipped when no source", ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "x:1", Policy: &RoutePolicy{Policy: trafficpolicy.Policy{Geo: &trafficpolicy.Geo{Mode: "deny", Countries: []string{"RU"}}}}}, false, "reverse_proxy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "s", ListenAddr: ":443", Routes: []ProxyRoute{tt.route}, RequestStats: tt.stats})
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(handlerNames(t, cfg.Apps.HTTP.Servers["s"].Routes[0].Handle), " ")
			if got != tt.expect {
				t.Fatalf("handlers = %q\nwant      %q", got, tt.expect)
			}
		})
	}
}

func TestPolicyCompile_HSTSOnlyWithRealTLS(t *testing.T) {
	h := trafficpolicy.Headers{Security: &trafficpolicy.SecurityPreset{HSTS: true}}
	tests := []struct {
		name string
		rp   RoutePolicy
		want string
	}{
		{"internal issuer drops hsts", RoutePolicy{}, ""},
		{"real tls gates on scheme", RoutePolicy{TLSReal: true}, "{http.request.scheme} == 'https'"},
		{"terminated upstream sets plainly", RoutePolicy{TLSTerminatedUpstream: true}, "Strict-Transport-Security"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, _ := json.Marshal(tt.rp.headerHandlers(h))
			if tt.want == "" {
				if strings.Contains(string(raw), "Strict-Transport-Security") {
					t.Fatalf("HSTS emitted without real TLS: %s", raw)
				}
				return
			}
			if !strings.Contains(string(raw), tt.want) {
				t.Fatalf("handlers %s missing %q", raw, tt.want)
			}
		})
	}
}
