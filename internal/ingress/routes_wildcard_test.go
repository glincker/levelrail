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
				DNSProvider: CloudflareDNSProvider{Name: "cloudflare", APIToken: "test-token"},
				Routes:      []ProxyRoute{wildcardRoute, regularRoute},
			},
			wantIssuers: 2,
			wantDNS01:   true,
		},
		{
			name: "wildcard host, ACME enabled, no dns provider: single plain ACME policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				Routes: []ProxyRoute{wildcardRoute, regularRoute},
			},
			wantIssuers: 1,
			wantDNS01:   false,
		},
		{
			name: "wildcard host, ACME disabled: internal issuer, unaffected by dns provider",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: false,
				DNSProvider: CloudflareDNSProvider{Name: "cloudflare", APIToken: "test-token"},
				Routes:      []ProxyRoute{wildcardRoute, regularRoute},
			},
			wantIssuers: 1,
			wantDNS01:   false,
		},
		{
			name: "no wildcard host, ACME enabled, cloudflare token set: single plain ACME policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				DNSProvider: CloudflareDNSProvider{Name: "cloudflare", APIToken: "test-token"},
				Routes:      []ProxyRoute{regularRoute},
			},
			wantIssuers: 1,
			wantDNS01:   false,
		},
		{
			name: "only wildcard hosts, ACME enabled, cloudflare token set: single DNS-01 policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				DNSProvider: CloudflareDNSProvider{Name: "cloudflare", APIToken: "test-token"},
				Routes:      []ProxyRoute{wildcardRoute},
			},
			wantIssuers: 1,
			wantDNS01:   true,
		},
		{
			name: "only wildcard hosts, ACME enabled, route53 credentials set: single DNS-01 policy",
			opts: RoutesOptions{
				ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true,
				DNSProvider: Route53DNSProvider{Name: "route53", AccessKeyID: "AKIA...", SecretAccessKey: "shh"},
				Routes:      []ProxyRoute{wildcardRoute},
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
			gotDNS01 := strings.Contains(string(raw), `"api_token"`) || strings.Contains(string(raw), `"access_key_id"`)
			if gotDNS01 != tt.wantDNS01 {
				t.Errorf("config contains a DNS-01 provider = %v, want %v\nconfig: %s", gotDNS01, tt.wantDNS01, raw)
			}
		})
	}
}

// TestNewDNSACMEIssuer_JSONShape asserts the marshaled shape at the
// specific field paths the DNS-01 automation policy relies on, for both
// supported providers: challenges.dns.provider.name plus each
// provider's own credential fields carried through untouched.
func TestNewDNSACMEIssuer_JSONShape(t *testing.T) {
	tests := []struct {
		name     string
		provider DNS01Provider
		want     map[string]string
	}{
		{
			name:     "cloudflare",
			provider: CloudflareDNSProvider{Name: "cloudflare", APIToken: "test-token"},
			want:     map[string]string{"name": "cloudflare", "api_token": "test-token"},
		},
		{
			name:     "route53",
			provider: Route53DNSProvider{Name: "route53", AccessKeyID: "AKIA-test", SecretAccessKey: "shh", Region: "us-east-1", HostedZoneID: "Z123"},
			want:     map[string]string{"name": "route53", "access_key_id": "AKIA-test", "secret_access_key": "shh", "region": "us-east-1", "hosted_zone_id": "Z123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iss := NewDNSACMEIssuer("ops@example.com", "", tt.provider)
			decoded := decodeIssuerJSON(t, iss)

			if decoded["module"] != "acme" {
				t.Errorf("module = %v, want \"acme\"", decoded["module"])
			}
			if decoded["email"] != "ops@example.com" {
				t.Errorf("email = %v, want \"ops@example.com\"", decoded["email"])
			}
			provider := decodeDNSProvider(t, decoded)
			for field, want := range tt.want {
				if provider[field] != want {
					t.Errorf("provider[%q] = %v, want %q", field, provider[field], want)
				}
			}
		})
	}
}

// decodeIssuerJSON marshals then unmarshals iss into a plain map,
// failing the test immediately on either error. Shared by every test in
// this file that asserts on an issuer's JSON field paths rather than its
// Go struct fields directly (the wire shape is the actual contract).
func decodeIssuerJSON(t *testing.T, iss ACMEIssuer) map[string]any {
	t.Helper()
	raw, err := json.Marshal(iss)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	return decoded
}

// decodeDNSProvider descends decoded's challenges.dns.provider path,
// failing the test immediately if any level is missing or the wrong
// shape.
func decodeDNSProvider(t *testing.T, decoded map[string]any) map[string]any {
	t.Helper()
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
	return provider
}
