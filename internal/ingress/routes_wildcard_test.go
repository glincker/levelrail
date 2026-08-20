package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildRoutesConfig_WildcardIssuerSelection covers the DNS-01-vs-
// HTTP-01 issuer selection: a wildcard host only gets Cloudflare DNS-01
// when both ACME is enabled and a Cloudflare API token is configured;
// every other combination reproduces this package's prior, non-wildcard
// behavior exactly.
func TestBuildRoutesConfig_WildcardIssuerSelection(t *testing.T) {
	wildcardRoute := ProxyRoute{Hosts: []string{"*.example.com"}, BackendDial: "127.0.0.1:9001"}
	regularRoute := ProxyRoute{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9002"}

	tests := []struct {
		name        string
		opts        RoutesOptions
		wantIssuers int // number of automation policies
		wantDNS01   bool
	}{
		{
			name: "wildcard host, ACME enabled, cloudflare token set: split policies with DNS-01",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				CloudflareDNSAPIToken: "test-token",
				Routes:                []ProxyRoute{wildcardRoute, regularRoute},
			},
			wantIssuers: 2,
			wantDNS01:   true,
		},
		{
			name: "wildcard host, ACME enabled, no cloudflare token: single plain ACME policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				Routes: []ProxyRoute{wildcardRoute, regularRoute},
			},
			wantIssuers: 1,
			wantDNS01:   false,
		},
		{
			name: "wildcard host, ACME disabled: internal issuer, unaffected by cloudflare token",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: false,
				CloudflareDNSAPIToken: "test-token",
				Routes:                []ProxyRoute{wildcardRoute, regularRoute},
			},
			wantIssuers: 1,
			wantDNS01:   false,
		},
		{
			name: "no wildcard host, ACME enabled, cloudflare token set: single plain ACME policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				CloudflareDNSAPIToken: "test-token",
				Routes:                []ProxyRoute{regularRoute},
			},
			wantIssuers: 1,
			wantDNS01:   false,
		},
		{
			name: "only wildcard hosts, ACME enabled, cloudflare token set: single DNS-01 policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				CloudflareDNSAPIToken: "test-token",
				Routes:                []ProxyRoute{wildcardRoute},
			},
			wantIssuers: 1,
			wantDNS01:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := BuildRoutesConfig(tt.opts)
			if err != nil {
				t.Fatalf("BuildRoutesConfig() error: %v", err)
			}
			if cfg.Apps.TLS == nil || cfg.Apps.TLS.Automation == nil {
				t.Fatalf("Apps.TLS.Automation is nil, want automation policies")
			}
			policies := cfg.Apps.TLS.Automation.Policies
			if len(policies) != tt.wantIssuers {
				t.Fatalf("len(policies) = %d, want %d: %+v", len(policies), tt.wantIssuers, policies)
			}

			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("json.Marshal() error: %v", err)
			}
			gotDNS01 := strings.Contains(string(raw), `"api_token"`)
			if gotDNS01 != tt.wantDNS01 {
				t.Errorf("config contains a cloudflare DNS-01 provider = %v, want %v\nconfig: %s", gotDNS01, tt.wantDNS01, raw)
			}
		})
	}
}

// TestNewCloudflareDNSACMEIssuer_JSONShape asserts the marshaled shape at
// the specific field paths the DNS-01 automation policy relies on:
// challenges.dns.provider.name == "cloudflare" and the api token carried
// through untouched.
func TestNewCloudflareDNSACMEIssuer_JSONShape(t *testing.T) {
	iss := NewCloudflareDNSACMEIssuer("ops@example.com", "", "test-token")

	raw, err := json.Marshal(iss)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if decoded["module"] != "acme" {
		t.Errorf("module = %v, want \"acme\"", decoded["module"])
	}
	if decoded["email"] != "ops@example.com" {
		t.Errorf("email = %v, want \"ops@example.com\"", decoded["email"])
	}
	challenges, ok := decoded["challenges"].(map[string]any)
	if !ok {
		t.Fatalf("challenges missing or wrong shape: %v", decoded["challenges"])
	}
	dns, ok := challenges["dns"].(map[string]any)
	if !ok {
		t.Fatalf("challenges.dns missing or wrong shape: %v", challenges["dns"])
	}
	provider, ok := dns["provider"].(map[string]any)
	if !ok {
		t.Fatalf("challenges.dns.provider missing or wrong shape: %v", dns["provider"])
	}
	if provider["name"] != "cloudflare" {
		t.Errorf("provider.name = %v, want \"cloudflare\"", provider["name"])
	}
	if provider["api_token"] != "test-token" {
		t.Errorf("provider.api_token = %v, want \"test-token\"", provider["api_token"])
	}
}
