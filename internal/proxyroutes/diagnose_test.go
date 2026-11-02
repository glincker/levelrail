package proxyroutes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestClassifyCert(t *testing.T) {
	tests := []struct {
		name                string
		cn, org, subject    string
		selfSigned, trusted bool
		want                string
	}{
		{"traefik default", "TRAEFIK DEFAULT CERT", "", "TRAEFIK DEFAULT CERT", true, false, ClassProxyDefault},
		{"caddy local", "Caddy Local Authority - ECC Intermediate", "Caddy Local Authority", "x", false, false, ClassProxyDefault},
		{"caddy root org", "Caddy Local Authority", "", "x", false, false, ClassProxyDefault},
		{"ingress fake", "Kubernetes Ingress Controller Fake Certificate", "Acme Co", "x", true, false, ClassProxyDefault},
		{"nginx localhost", "localhost", "", "localhost", true, false, ClassProxyDefault},
		{"cloudflare origin", "CloudFlare Origin SSL Certificate Authority", "CloudFlare, Inc.", "x", false, false, ClassOriginCA},
		{"lets encrypt", "R11", "Let's Encrypt", "x", false, true, ClassTrusted},
		{"corporate ca", "Corp Issuing CA", "Corp", "x", false, false, ClassPrivateCA},
		{"trusted wins over name", "TRAEFIK DEFAULT CERT", "", "x", true, true, ClassTrusted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyCert(tc.cn, tc.org, tc.subject, tc.selfSigned, tc.trusted); got != tc.want {
				t.Errorf("ClassifyCert = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDiagnose(t *testing.T) {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	yes, no := true, false
	base := DiagnoseInput{
		Domain: "app.example.com", Class: ClassProxyDefault, Issuer: "TRAEFIK DEFAULT CERT", Now: now, FirstSeen: now.Add(-time.Minute),
		Budget: 5 * time.Minute, RouterName: "acme-app", Resolvers: []string{"letsencrypt"},
	}
	with := func(f func(*DiagnoseInput)) DiagnoseInput { in := base; f(&in); return in }
	tests := []struct {
		name     string
		in       DiagnoseInput
		want     string
		contains string
	}{
		{"healthy", with(func(i *DiagnoseInput) { i.Class = ClassTrusted; i.NotAfter = now.Add(time.Hour) }), DiagNone, ""},
		{"expired", with(func(i *DiagnoseInput) { i.Class = ClassTrusted; i.NotAfter = now.Add(-time.Hour) }), DiagCertExpired, ""},
		{"origin", with(func(i *DiagnoseInput) { i.Class = ClassOriginCA }), DiagOriginCert, ""},
		{"private", with(func(i *DiagnoseInput) { i.Class = ClassPrivateCA }), DiagPrivateCA, ""},
		{"no tls", with(func(i *DiagnoseInput) { i.Router = &RouterTLS{Found: true} }), DiagNoTLS, "tls=true"},
		{"no resolver", with(func(i *DiagnoseInput) { i.Router = &RouterTLS{Found: true, HasTLS: true} }), DiagNoResolver, "tls.certresolver=letsencrypt"},
		{"no resolver, none known", with(func(i *DiagnoseInput) { i.Router = &RouterTLS{Found: true, HasTLS: true}; i.Resolvers = nil }), DiagNoResolver, "<resolver>"},
		{"dns elsewhere", with(func(i *DiagnoseInput) {
			i.Router = &RouterTLS{Found: true, HasTLS: true, CertResolver: "letsencrypt"}
			i.DNS = &DNSFacts{IPs: []string{"203.0.113.9"}, ServerIPs: []string{"198.51.100.4"}}
		}), DiagDNSMismatch, "198.51.100.4"},
		{"dns proxied", with(func(i *DiagnoseInput) {
			i.DNS = &DNSFacts{IPs: []string{"104.21.5.5"}, ServerIPs: []string{"198.51.100.4"}}
		}), DiagDNSMismatch, "CDN"},
		{"dns lookup fails", with(func(i *DiagnoseInput) { i.DNS = &DNSFacts{Err: "no such host"} }), DiagDNSMismatch, "no such host"},
		{"dns ok, port closed", with(func(i *DiagnoseInput) {
			i.DNS = &DNSFacts{IPs: []string{"198.51.100.4"}, ServerIPs: []string{"198.51.100.4"}}
			i.Port80Open = &no
		}), DiagPortUnreachable, "port 80"},
		{"pending", with(func(i *DiagnoseInput) { i.Port80Open = &yes }), DiagACMEPending, ""},
		{"stuck after budget", with(func(i *DiagnoseInput) { i.FirstSeen = now.Add(-time.Hour) }), DiagACMEStuck, "no certificate after"},
		{"log line", with(func(i *DiagnoseInput) {
			i.ACMELog = "Unable to obtain ACME certificate for domains app.example.com: acme: error: 403"
		}), DiagACMEFailed, "proxy log"},
		{"rate limit", with(func(i *DiagnoseInput) { i.ACMELog = "acme: error: 429 :: urn:ietf:params:acme:error:rateLimited" }), DiagACMERateLimited, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Diagnose(tc.in)
			if d.Code != tc.want {
				t.Fatalf("code = %q, want %q (%+v)", d.Code, tc.want, d)
			}
			all := d.Summary + d.Hint + d.Snippet + strings.Join(d.Evidence, "\n")
			if !strings.Contains(all, tc.contains) {
				t.Errorf("diagnosis lacks %q: %+v", tc.contains, d)
			}
		})
	}
}

func TestACMEFailure(t *testing.T) {
	logs := strings.Join([]string{
		`time="x" level=error msg="Unable to obtain ACME certificate for domains \"other.example.com\""`,
		`time="x" level=error msg="Unable to obtain ACME certificate for domains \"App.Example.com\": acme: error: 403"`,
		`time="x" level=info msg="app.example.com is fine"`,
	}, "\n")
	if got := ACMEFailure(logs, "app.example.com"); !strings.Contains(got, "403") {
		t.Errorf("got %q", got)
	}
	if got := ACMEFailure(logs, "none.example.com"); got != "" {
		t.Errorf("unrelated domain matched: %q", got)
	}
	if got := ACMEFailure(strings.Repeat("x", 900)+" app.example.com acme: error", "app.example.com"); len(got) > maxACMELogLine+3 {
		t.Errorf("not trimmed: %d", len(got))
	}
}

func TestTraefikAPIEvidence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/http/routers/with":
			_, _ = w.Write([]byte(`{"status":"enabled","tls":{"certResolver":"le","domains":[{"main":"a.example.com"}]}}`))
		case "/api/http/routers/bare":
			_, _ = w.Write([]byte(`{"status":"enabled","tls":{}}`))
		case "/api/http/routers/plain":
			_, _ = w.Write([]byte(`{"status":"enabled"}`))
		case "/api/http/routers":
			_, _ = w.Write([]byte(`[{"tls":{"certResolver":"le"}},{"tls":{"certResolver":"other"}},{"tls":{"certResolver":"le"}},{}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	got, err := RouterTLSInfo(ctx, srv.URL, "with", time.Second)
	if err != nil || !got.Found || got.CertResolver != "le" || got.Domains[0] != "a.example.com" {
		t.Errorf("with = %+v %v", got, err)
	}
	if got, _ := RouterTLSInfo(ctx, srv.URL, "bare", time.Second); !got.HasTLS || got.CertResolver != "" {
		t.Errorf("bare = %+v", got)
	}
	if got, _ := RouterTLSInfo(ctx, srv.URL, "plain", time.Second); !got.Found || got.HasTLS {
		t.Errorf("plain = %+v", got)
	}
	if got, _ := RouterTLSInfo(ctx, srv.URL, "gone", time.Second); got.Found {
		t.Errorf("gone = %+v", got)
	}
	names, err := KnownResolvers(ctx, srv.URL, time.Second)
	if err != nil || len(names) != 2 || names[0] != "le" {
		t.Errorf("resolvers = %v %v", names, err)
	}
}

func TestSyncerRetryNudgesThenRestores(t *testing.T) {
	dir := t.TempDir()
	st := newFakeSyncStore(dir)
	st.services = []store.DesiredService{{Name: "web", Domains: []string{"a.example.com"}}}
	s := newTestSyncer(st)
	s.RetryGap = 150 * time.Millisecond
	ctx := context.Background()
	if _, err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "acme-managed-a.example.com.yaml")
	want, err := os.ReadFile(file) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	var sawNudge atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			if b, _ := os.ReadFile(file); strings.Contains(string(b), "priority: 100001") { //nolint:gosec // test temp path
				sawNudge.Store(true)
				return
			}
		}
	}()
	if err := s.Retry(ctx, "a.example.com"); err != nil {
		t.Fatal(err)
	}
	<-done
	if !sawNudge.Load() {
		t.Error("file never carried the nudged priority")
	}
	if got, _ := os.ReadFile(file); string(got) != string(want) { //nolint:gosec // test temp path
		t.Errorf("file not restored:\n%s", got)
	}
	if err := s.Retry(ctx, "missing.example.com"); err == nil {
		t.Error("unrouted domain accepted")
	}
}
