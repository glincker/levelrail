package ingress

import (
	"encoding/json"
	"reflect"
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
			got := NewACMEIssuer(tt.email, tt.directoryURL)
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
	want := NewACMEIssuer("ops@example.com", "https://acme-staging-v02.api.letsencrypt.org/directory")
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
