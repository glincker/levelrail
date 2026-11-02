package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
)

func fallbackProbe(_ context.Context, _ proxyroutes.ProbeTarget) proxyroutes.ProbeResult {
	return proxyroutes.ProbeResult{
		Reachable: true, StatusCode: 200, CertIssuer: "TRAEFIK DEFAULT CERT", CertClass: proxyroutes.ClassProxyDefault,
		CertNotAfter: time.Now().Add(365 * 24 * time.Hour), Err: "generic",
	}
}

func TestProxyVerifyDiagnosesFallbackCertificate(t *testing.T) {
	env := newProxyTestEnv(t)
	d := env.rt.proxyIntegration
	d.probe = fallbackProbe
	d.routerTLS = func(context.Context, string, string, time.Duration) (proxyroutes.RouterTLS, error) {
		return proxyroutes.RouterTLS{Found: true, HasTLS: true, CertResolver: "letsencrypt"}, nil
	}
	d.knownResolvers = func(context.Context, string, time.Duration) ([]string, error) { return []string{"letsencrypt"}, nil }
	d.lookupIPs = func(context.Context, string) ([]string, error) { return []string{"203.0.113.10"}, nil }
	open := true
	d.dialOpen = func(context.Context, string) bool { return open }
	d.proxyLogs = func(context.Context, string) (string, error) {
		return `level=error msg="Unable to obtain ACME certificate for domains \"app.example.com\": acme: error: 403 token=abc"`, nil
	}
	if code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`); code != http.StatusOK {
		t.Fatalf("setup: %d %s", code, body)
	}
	read := func() proxyDomainResource {
		code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/verify?domain=app.example.com", "")
		var res proxyIntegrationResource
		if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK || len(res.Domains) != 1 {
			t.Fatalf("verify: %d %s", code, body)
		}
		return res.Domains[0]
	}
	got := read()
	if got.Diagnosis != proxyroutes.DiagACMEFailed || got.Certificate == nil || got.Certificate.Trusted || got.Certificate.Expired {
		t.Fatalf("domain = %+v cert=%+v", got, got.Certificate)
	}
	if !strings.Contains(strings.Join(got.Evidence, "\n"), "403") || got.LastError == "generic" {
		t.Errorf("evidence/last error = %v / %q", got.Evidence, got.LastError)
	}
	d.proxyLogs = nil
	d.dialOpen = func(context.Context, string) bool { return false }
	if got := read(); got.Diagnosis != proxyroutes.DiagPortUnreachable {
		t.Errorf("without logs and port closed: %s", got.Diagnosis)
	}
	d.dialOpen = func(context.Context, string) bool { return true }
	got = read()
	if got.Diagnosis != proxyroutes.DiagACMEPending || got.RecheckUntil == "" {
		t.Errorf("fresh route should be pending with a recheck window: %+v", got)
	}
}

func TestProxyVerifyRetryNudgesProxy(t *testing.T) {
	env := newProxyTestEnv(t)
	var retried []string
	env.rt.proxyIntegration.retryCert = func(_ context.Context, domain string) error { retried = append(retried, domain); return nil }
	if code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`); code != http.StatusOK {
		t.Fatalf("setup: %d %s", code, body)
	}
	if code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/verify?domain=app.example.com&retry=1", ""); code != http.StatusOK {
		t.Fatalf("retry: %d %s", code, body)
	}
	if len(retried) != 1 || retried[0] != "app.example.com" {
		t.Errorf("retried = %v", retried)
	}
}
