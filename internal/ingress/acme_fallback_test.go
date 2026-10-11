package ingress

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidateACMEContact(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  error
	}{
		{name: "real address", email: "ops@glincker.com"},
		{name: "subdomain address", email: "ops@mail.glincker.com"},
		{name: "missing at", email: "not-an-email", want: ErrACMEContactInvalid},
		{name: "bare at", email: "@", want: ErrACMEContactInvalid},
		{name: "display name", email: "Ops <ops@glincker.com>", want: ErrACMEContactInvalid},
		{name: "no dot in domain", email: "ops@corp", want: ErrACMEContactInvalid},
		{name: "ip literal domain", email: "ops@[10.0.0.1]", want: ErrACMEContactInvalid},
		{name: "example.com", email: "ops@example.com", want: ErrACMEContactReserved},
		{name: "example.org subdomain", email: "ops@mail.example.org", want: ErrACMEContactReserved},
		{name: "test tld", email: "ops@corp.test", want: ErrACMEContactReserved},
		{name: "invalid tld", email: "ops@corp.invalid", want: ErrACMEContactReserved},
		{name: "localhost tld", email: "ops@box.localhost", want: ErrACMEContactReserved},
		{name: "uppercase reserved", email: "ops@EXAMPLE.COM", want: ErrACMEContactReserved},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateACMEContact(tc.email); !errors.Is(got, tc.want) {
				t.Fatalf("ValidateACMEContact(%q) = %v, want %v", tc.email, got, tc.want)
			}
		})
	}
}

func TestACMEIssuable(t *testing.T) {
	tests := map[string]bool{
		"app.glincker.com":     true,
		"*.glincker.com":       true,
		"203-0-113-7.sslip.io": true,
		"app.example.com":      false,
		"example.org":          false,
		"foo.test":             false,
		"foo.invalid":          false,
		"app.localhost":        false,
		"localhost":            false,
		"nas.local":            false,
		"db.internal":          false,
		"10.0.0.1":             false,
		"intranet":             false,
		"":                     false,
	}
	for host, want := range tests {
		if got := ACMEIssuable(host); got != want {
			t.Errorf("ACMEIssuable(%q) = %v, want %v", host, got, want)
		}
	}
}

func acmeFailedOpts() RoutesOptions {
	return RoutesOptions{
		ServerName: "ingress", ListenAddr: ":443", TLS: true, HTTPRedirect: true,
		ACMEEnabled: true, ACMEEmail: "ops@example.com", FallbackCertLifetime: "6h0m0s",
		Routes: []ProxyRoute{
			{Hosts: []string{"web.glincker.com"}, BackendDial: "127.0.0.1:9001"},
			{Hosts: []string{"api.glincker.com"}, BackendDial: "127.0.0.1:9002"},
		},
	}
}

// With a contact the CA rejects, the config must still end every host's issuer
// chain in the internal issuer, so the listener never has "no certificate".
func TestBuildRoutesConfig_FailedIssuanceKeepsInternalFallback(t *testing.T) {
	cfg, err := BuildRoutesConfig(acmeFailedOpts())
	if err != nil {
		t.Fatalf("BuildRoutesConfig() error = %v", err)
	}
	if cfg.Apps.PKI == nil || cfg.Apps.PKI.CertificateAuthorities["local"] == nil {
		t.Fatal("PKI app missing: the internal fallback needs its local CA configured")
	}
	if cfg.Logging == nil || cfg.Logging.Logs["acme_tap"] == nil {
		t.Fatal("ACME log tap missing: issuance errors would never reach the domain status")
	}
	policies := cfg.Apps.TLS.Automation.Policies
	for _, p := range policies {
		if len(p.Issuers) < 2 {
			t.Fatalf("policy %v issuers = %+v, want public issuer then internal", p.Subjects, p.Issuers)
		}
		last, ok := p.Issuers[len(p.Issuers)-1].(InternalIssuer)
		if !ok {
			t.Fatalf("policy %v last issuer = %T, want InternalIssuer", p.Subjects, p.Issuers[len(p.Issuers)-1])
		}
		if last.Lifetime != "6h0m0s" {
			t.Errorf("fallback lifetime = %q, want 6h0m0s", last.Lifetime)
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{`"module":"acme"`, `"module":"internal"`, `"lifetime":"6h0m0s"`, `"output":"acme_tap"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("config JSON missing %s: %s", want, raw)
		}
	}
}

// Caddy's automatic redirect covers exactly the hosts in a policy, so the
// regression to guard is a redirecting config with a host that no policy,
// or a policy with no issuer, would leave without a certificate.
func TestBuildRoutesConfig_RedirectOnlyForHostsWithAnIssuer(t *testing.T) {
	tests := []struct {
		name string
		opts func() RoutesOptions
	}{
		{name: "acme enabled", opts: acmeFailedOpts},
		{name: "acme disabled", opts: func() RoutesOptions {
			o := acmeFailedOpts()
			o.ACMEEnabled = false
			return o
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := BuildRoutesConfig(tc.opts())
			if err != nil {
				t.Fatalf("BuildRoutesConfig() error = %v", err)
			}
			server := cfg.Apps.HTTP.Servers["ingress"]
			if server.AutomaticHTTPS != nil && server.AutomaticHTTPS.DisableRedir {
				t.Fatal("redirect disabled: this case must redirect")
			}
			covered := map[string]bool{}
			for _, p := range cfg.Apps.TLS.Automation.Policies {
				if len(p.Issuers) == 0 {
					t.Fatalf("policy %v has no issuer, its hosts would redirect to a certificate-less listener", p.Subjects)
				}
				for _, s := range p.Subjects {
					covered[s] = true
				}
			}
			for _, h := range ServedHosts(cfg) {
				if !covered[h] {
					t.Errorf("host %s is routed (and redirected) but has no certificate policy", h)
				}
			}
		})
	}
}

func TestRecordACMELogLine(t *testing.T) {
	defaultACMEFailures.ClearAll()
	t.Cleanup(defaultACMEFailures.ClearAll)

	line := func(msg, issuer string) []byte {
		b, _ := json.Marshal(map[string]string{
			"msg": msg, "identifier": "Web.Glincker.com", "issuer": issuer,
			"error": "urn:ietf:params:acme:error:invalidContact: invalid contact domain",
		})
		return b
	}
	tests := []struct {
		name string
		line []byte
		want bool
	}{
		{name: "acme issuer failure", line: line("could not get certificate from issuer", "acme-staging-v02.api.letsencrypt.org-directory"), want: true},
		{name: "internal issuer failure ignored", line: line("could not get certificate from issuer", "local")},
		{name: "unrelated message", line: line("served key authentication", "acme")},
		{name: "garbage", line: []byte("not json")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defaultACMEFailures.ClearAll()
			recordACMELogLine(tc.line)
			f, ok := defaultACMEFailures.Get("web.glincker.com")
			if ok != tc.want {
				t.Fatalf("recorded = %v, want %v", ok, tc.want)
			}
			if ok && !strings.Contains(f.Error, "invalidContact") {
				t.Errorf("error = %q, want the CA's own text", f.Error)
			}
		})
	}
}
