package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDemoAnalyticsFromEnvNeedsBoth(t *testing.T) {
	t.Parallel()
	host, tok := "APP_DEMO_ANALYTICS_HOST", "APP_DEMO_ANALYTICS_TOKEN"
	if demoAnalyticsFromEnv(envMap(map[string]string{host: "https://us.i.posthog.com"})) != nil {
		t.Error("host alone must not enable analytics")
	}
	if demoAnalyticsFromEnv(envMap(map[string]string{tok: "phc_x"})) != nil {
		t.Error("token alone must not enable analytics")
	}
	if demoAnalyticsFromEnv(envMap(map[string]string{host: "http://insecure.example", tok: "phc_x"})) != nil {
		t.Error("a non-https host must be refused")
	}
	a := demoAnalyticsFromEnv(envMap(map[string]string{host: "https://us.i.posthog.com/", tok: "phc_x"}))
	if a == nil || a.info.Host != "https://us.i.posthog.com" || a.info.Provider != "posthog" || a.info.Token != "phc_x" {
		t.Fatalf("unexpected analytics %+v", a)
	}
	if !strings.Contains(a.csp, "connect-src 'self' https://us.i.posthog.com") || !strings.Contains(a.csp, "https://us-assets.i.posthog.com") {
		t.Errorf("csp missing analytics hosts: %s", a.csp)
	}
}

func TestDemoInfoAnalyticsPayloadAndCSP(t *testing.T) {
	t.Parallel()
	rt := demoRouter(t, DemoConfig{Username: "demo"})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo", nil))
	if strings.Contains(rec.Body.String(), "analytics") {
		t.Errorf("analytics must be absent without env: %s", rec.Body.String())
	}
	rt.demo.analytics = demoAnalyticsFromEnv(envMap(map[string]string{
		"APP_DEMO_ANALYTICS_HOST": "https://eu.example.com", "APP_DEMO_ANALYTICS_TOKEN": "phc_x",
	}))
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo", nil))
	if !strings.Contains(rec.Body.String(), `"analytics":{"provider":"posthog","host":"https://eu.example.com","token":"phc_x"}`) {
		t.Errorf("payload missing analytics: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "connect-src 'self' https://eu.example.com") {
		t.Errorf("csp not widened: %s", rec.Header().Get("Content-Security-Policy"))
	}
}

func TestDemoGTM(t *testing.T) {
	t.Parallel()
	if demoGTMFromEnv(envMap(nil)) != nil {
		t.Error("unset must disable gtm")
	}
	for _, bad := range []string{"GTM-abc", "GTM-AB", "UA-12345", "GTM-ABCD\";x", "GTM-ABCDEFGHIJKLM"} {
		if demoGTMFromEnv(envMap(map[string]string{"APP_DEMO_GTM_ID": bad})) != nil {
			t.Errorf("%q must be ignored", bad)
		}
	}
	rt := demoRouter(t, DemoConfig{Username: "demo"})
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo", nil))
		return rec
	}
	rec := get()
	if strings.Contains(rec.Body.String(), "gtm") || strings.Contains(rec.Header().Get("Content-Security-Policy"), "googletagmanager") {
		t.Errorf("gtm must be absent without env: %s", rec.Body.String())
	}
	rt.demo.gtm = demoGTMFromEnv(envMap(map[string]string{"APP_DEMO_GTM_ID": "GTM-ABC123"}))
	rec = get()
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(rec.Body.String(), `"gtm":{"id":"GTM-ABC123"}`) {
		t.Errorf("payload missing gtm: %s", rec.Body.String())
	}
	for _, want := range []string{"script-src 'self' https://www.googletagmanager.com;", "https://*.google-analytics.com", "https://*.analytics.google.com"} {
		if !strings.Contains(csp, want) {
			t.Errorf("csp missing %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-eval") || strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Errorf("csp too loose: %s", csp)
	}
}
