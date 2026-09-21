package ingress

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestNewACMEIssuer_ConfigShape is a pure struct-construction test: no
// Caddy instance is started, this only checks the JSON this package
// would hand to caddy.Load matches the real tls.issuance.acme module's
// contract (verified against the vendored caddyserver/caddy/v2 source
// this module pins, modules/caddytls/acmeissuer.go's ACMEIssuer struct:
// json tags "module" (via this package's own Module field, the same
// discriminator convention InternalIssuer already uses), "email", and
// "ca"). See internal/api/certificates.go and ADR 005's own doc comment
// for why this field only closes a documented, expected gap rather than
// being a new architecture decision.
func TestNewACMEIssuer_ConfigShape(t *testing.T) {
	tests := []struct {
		name         string
		email        string
		directoryURL string
		want         ACMEIssuer
	}{
		{
			name:  "email only, no directory override",
			email: "ops@example.com",
			want:  ACMEIssuer{Module: "acme", Email: "ops@example.com"},
		},
		{
			name:         "email and staging directory override",
			email:        "ops@example.com",
			directoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
			want: ACMEIssuer{
				Module: "acme",
				Email:  "ops@example.com",
				CA:     "https://acme-staging-v02.api.letsencrypt.org/directory",
			},
		},
		{
			name: "empty email accepted structurally, validation is internal/api's job",
			want: ACMEIssuer{Module: "acme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewACMEIssuer(tt.email, tt.directoryURL, 0)
			if got != tt.want {
				t.Fatalf("NewACMEIssuer(%q, %q) = %+v, want %+v", tt.email, tt.directoryURL, got, tt.want)
			}

			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if decoded["module"] != "acme" {
				t.Errorf(`decoded["module"] = %v, want "acme"`, decoded["module"])
			}
			if tt.email == "" {
				if _, ok := decoded["email"]; ok {
					t.Errorf("empty Email should be omitted from JSON (omitempty), got %v", decoded["email"])
				}
			} else if decoded["email"] != tt.email {
				t.Errorf(`decoded["email"] = %v, want %q`, decoded["email"], tt.email)
			}
			if tt.directoryURL == "" {
				if _, ok := decoded["ca"]; ok {
					t.Errorf("empty CA should be omitted from JSON (omitempty), got %v", decoded["ca"])
				}
			} else if decoded["ca"] != tt.directoryURL {
				t.Errorf(`decoded["ca"] = %v, want %q`, decoded["ca"], tt.directoryURL)
			}
		})
	}
}

// TestNewACMEIssuer_AltHTTPPort proves altHTTPPort threads into
// Challenges.HTTP.AlternatePort only when it's a real override (non-zero
// and not Caddy's own default of 80): this is the one place real ACME's
// HTTP-01 challenge would otherwise reach for a literal port 80,
// independent of anything else this package makes configurable (see
// HTTPChallengeConfig's own doc comment).
func TestNewACMEIssuer_AltHTTPPort(t *testing.T) {
	tests := []struct {
		name        string
		altHTTPPort int
		wantNil     bool
		wantPort    int
	}{
		{name: "zero (unset) leaves Challenges nil", altHTTPPort: 0, wantNil: true},
		{name: "80 (Caddy's own default) leaves Challenges nil", altHTTPPort: 80, wantNil: true},
		{name: "non-default port sets AlternatePort", altHTTPPort: 8080, wantPort: 8080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iss := NewACMEIssuer("ops@example.com", "", tt.altHTTPPort)
			if tt.wantNil {
				if iss.Challenges != nil {
					t.Fatalf("Challenges = %+v, want nil", iss.Challenges)
				}
				return
			}
			if iss.Challenges == nil || iss.Challenges.HTTP == nil {
				t.Fatalf("Challenges = %+v, want a non-nil HTTP challenge config", iss.Challenges)
			}
			if iss.Challenges.HTTP.AlternatePort != tt.wantPort {
				t.Errorf("AlternatePort = %d, want %d", iss.Challenges.HTTP.AlternatePort, tt.wantPort)
			}

			raw, err := json.Marshal(iss)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if !strings.Contains(string(raw), `"alternate_port":8080`) {
				t.Errorf("marshaled issuer = %s, want alternate_port:8080", raw)
			}
		})
	}
}

// TestBuildRoutesConfig_HTTPPort proves opts.HTTPPort threads into both
// Apps.HTTP.HTTPPort (the top-level Caddy setting) and, for a
// non-wildcard host with real ACME enabled, the ACME issuer's own
// Challenges.HTTP.AlternatePort: the two places a control-plane instance
// configured off the default ports needs to stay off port 80 in.
func TestBuildRoutesConfig_HTTPPort(t *testing.T) {
	got, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "levelrail-ingress",
		ListenAddr: ":8443",
		HTTPPort:   8080,
		Routes: []ProxyRoute{
			{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:11111"},
		},
		TLS:              true,
		ACMEEnabled:      true,
		ACMEEmail:        "ops@example.com",
		ACMEDirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}

	if got.Apps.HTTP.HTTPPort != 8080 {
		t.Errorf("Apps.HTTP.HTTPPort = %d, want 8080", got.Apps.HTTP.HTTPPort)
	}
	issuer, ok := got.Apps.TLS.Automation.Policies[0].Issuers[0].(ACMEIssuer)
	if !ok {
		t.Fatalf("Issuers[0] = %T, want ACMEIssuer", got.Apps.TLS.Automation.Policies[0].Issuers[0])
	}
	if issuer.Challenges == nil || issuer.Challenges.HTTP == nil || issuer.Challenges.HTTP.AlternatePort != 8080 {
		t.Errorf("issuer.Challenges = %+v, want HTTP.AlternatePort=8080", issuer.Challenges)
	}
}

// TestBuildRoutesConfig_ACMEDisabled_RegressionUnchanged is the
// regression test the task's own safety constraints call out as the
// single most important one: with ACMEEnabled left at its zero value
// (false, matching every RoutesOptions literal that existed before this
// field did), BuildRoutesConfig's output must be identical, field for
// field, to what it produced before ACME support existed. Constructed as
// a direct reflect.DeepEqual against a config built with today's
// internal-issuer-only helpers, not merely "no error", so a silent
// change to the internal-issuer branch's shape would fail this test.
func TestBuildRoutesConfig_ACMEDisabled_RegressionUnchanged(t *testing.T) {
	opts := RoutesOptions{
		ServerName: "levelrail-ingress",
		ListenAddr: ":443",
		Routes: []ProxyRoute{
			{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:11111"},
		},
		StaticRoutes: []StaticRoute{
			{Hosts: []string{"static.example.com"}, RootDir: "/var/lib/levelrail-data/static/docs/abc"},
		},
		TLS: true,
		// ACMEEnabled deliberately left unset (false), ACMEEmail/
		// ACMEDirectoryURL deliberately left empty: this is exactly the
		// call shape every existing caller (internal/reconcile/ingress,
		// before this feature) makes.
	}

	got, err := BuildRoutesConfig(opts)
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}

	want := &Config{
		Admin: newAdminConfig(""),
		Apps: Apps{
			HTTP: &HTTPApp{
				Servers: map[string]*Server{
					"levelrail-ingress": {
						Listen: []string{":443"},
						Routes: []Route{
							{
								Match:  []Matcher{{Host: []string{"app.example.com"}}},
								Handle: []any{NewReverseProxyHandler("127.0.0.1:11111")},
							},
							{
								Match:  []Matcher{{Host: []string{"static.example.com"}}},
								Handle: []any{NewFileServerHandler("/var/lib/levelrail-data/static/docs/abc")},
							},
						},
						AutomaticHTTPS: &AutoHTTPSConfig{DisableRedir: true},
					},
				},
			},
			TLS: internalIssuerTLSApp([]string{"app.example.com", "static.example.com"}),
			PKI: newInternalPKIApp(),
		},
	}

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("BuildRoutesConfig() with ACME disabled changed shape.\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

// TestBuildRoutesConfig_ACMEEnabled covers the branch this feature adds:
// ACMEEnabled swaps the internal issuer for the real ACME issuer, scoped
// to every currently-routed host (both container-backed and static
// routes contribute to the same Subjects list, matching allHosts'
// existing "both kinds feed one automation policy" behavior), and
// deliberately builds no PKI app: real ACME has no local CA for Caddy's
// pki app to manage trust for.
func TestBuildRoutesConfig_ACMEEnabled(t *testing.T) {
	opts := RoutesOptions{
		ServerName: "levelrail-ingress",
		ListenAddr: ":443",
		Routes: []ProxyRoute{
			{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:11111"},
		},
		TLS:              true,
		ACMEEnabled:      true,
		ACMEEmail:        "ops@example.com",
		ACMEDirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",
	}

	got, err := BuildRoutesConfig(opts)
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}

	if got.Apps.PKI != nil {
		t.Errorf("Apps.PKI = %+v, want nil: ACME issuance needs no local CA", got.Apps.PKI)
	}
	if got.Apps.TLS == nil || got.Apps.TLS.Automation == nil || len(got.Apps.TLS.Automation.Policies) != 1 {
		t.Fatalf("Apps.TLS = %+v, want exactly one automation policy", got.Apps.TLS)
	}
	policy := got.Apps.TLS.Automation.Policies[0]
	if !reflect.DeepEqual(policy.Subjects, []string{"app.example.com"}) {
		t.Errorf("policy.Subjects = %v, want [app.example.com]", policy.Subjects)
	}
	if len(policy.Issuers) != 1 {
		t.Fatalf("policy.Issuers = %+v, want exactly one issuer", policy.Issuers)
	}
	issuer, ok := policy.Issuers[0].(ACMEIssuer)
	if !ok {
		t.Fatalf("policy.Issuers[0] = %T, want ACMEIssuer", policy.Issuers[0])
	}
	want := NewACMEIssuer("ops@example.com", "https://acme-staging-v02.api.letsencrypt.org/directory", 0)
	if issuer != want {
		t.Errorf("issuer = %+v, want %+v", issuer, want)
	}
}

// TestBuildRoutesConfig_ACMEEnabled_NoHosts proves ACMEEnabled alone
// doesn't force a TLS app into existence: the existing "TLS && at least
// one host" guard in BuildRoutesConfig still governs whether any
// automation policy is built at all, ACME or internal, matching the
// zero-routes-is-a-valid-reconcile-pass shape this package already
// documents.
func TestBuildRoutesConfig_ACMEEnabled_NoHosts(t *testing.T) {
	got, err := BuildRoutesConfig(RoutesOptions{
		ServerName:  "levelrail-ingress",
		ListenAddr:  ":443",
		TLS:         true,
		ACMEEnabled: true,
		ACMEEmail:   "ops@example.com",
	})
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}
	if got.Apps.TLS != nil {
		t.Errorf("Apps.TLS = %+v, want nil with zero routable hosts", got.Apps.TLS)
	}
}
