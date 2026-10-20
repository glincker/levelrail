package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type proxyCall struct{ method, uri, body string }

func newProxyIntegrationServer(t *testing.T, calls *[]proxyCall) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*calls = append(*calls, proxyCall{r.Method, r.URL.RequestURI(), string(b)})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/system/reverse-proxy" {
			_ = json.NewEncoder(w).Encode(map[string]any{"holders": []any{}, "plan": map[string]any{"domain": "app.example.com", "proxy": "nginx", "snippet": "server {}", "steps": []string{"one"}}})
			return
		}
		if r.URL.Path == "/api/v1/settings/proxy-integration" {
			_, _ = w.Write(b)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"detected": map[string]any{"kind": "traefik", "container": "coolify-proxy", "image": "traefik:v3.6", "published_ports": []int{80, 443}, "dynamic_dir": "/data/coolify/proxy/dynamic", "complete": true, "missing": []string{}},
			"settings": map[string]any{"integration": "traefik_file", "dynamic_dir": "/data/coolify/proxy/dynamic", "entrypoint_http": "http", "entrypoint_https": "https", "cert_resolver": "letsencrypt", "upstream_host": "host.docker.internal"},
			"domains":  []any{map[string]any{"domain": "app.example.com", "target": "app", "app": "web", "state": "written", "reachable": true, "checked_at": "2026-10-10T00:00:00Z", "certificate": map[string]any{"issuer": "R11", "not_after": "2027-01-01T00:00:00Z", "valid": true}}},
			"steps":    []any{map[string]any{"id": "routes", "state": "done", "detail": "1 route files written"}},
			"dry_run":  r.URL.Path == "/api/v1/system/proxy-integration/setup" && !strings.Contains(string(b), `"confirm":true`),
			"changes":  []string{"integration: off -> traefik_file"},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProxyIntegrationCLI(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantCalls []proxyCall
		wantOut   []string
	}{
		{"status", []string{"proxy", "status"}, []proxyCall{{"GET", "/api/v1/system/proxy-integration", ""}},
			[]string{"proxy: traefik in container coolify-proxy", "app.example.com -> app web [written] reachable=true cert=R11", "[done] routes"}},
		{"setup dry run", []string{"proxy", "setup"}, []proxyCall{{"POST", "/api/v1/system/proxy-integration/setup", `{"confirm":false}`}},
			[]string{"setup would change integration: off -> traefik_file", "proxy setup --confirm"}},
		{"setup confirm", []string{"proxy", "setup", "--confirm", "--dynamic-dir", "/srv/dyn"}, []proxyCall{{"POST", "/api/v1/system/proxy-integration/setup", `{"confirm":true,"dynamic_dir":"/srv/dyn"}`}},
			[]string{"setup changed integration"}},
		{"apply", []string{"proxy", "apply"}, []proxyCall{{"POST", "/api/v1/system/proxy-integration/apply", ""}}, []string{"integration: traefik_file"}},
		{"verify one", []string{"proxy", "verify", "--domain", "app.example.com"}, []proxyCall{{"POST", "/api/v1/system/proxy-integration/verify?domain=app.example.com", ""}}, []string{"reachable=true"}},
		{"disable keeps the saved values", []string{"proxy", "disable"}, []proxyCall{
			{"GET", "/api/v1/system/proxy-integration", ""},
			{"PUT", "/api/v1/settings/proxy-integration", `{"integration":"off","dynamic_dir":"/data/coolify/proxy/dynamic","entrypoint_http":"http","entrypoint_https":"https","cert_resolver":"letsencrypt","upstream_host":"host.docker.internal"}`},
			{"GET", "/api/v1/system/proxy-integration", ""},
		}, nil},
		{"guide for an app", []string{"proxy", "--app", "web", "--proxy", "nginx"}, []proxyCall{{"GET", "/api/v1/system/reverse-proxy?app=web&proxy=nginx", ""}}, []string{"Put app.example.com behind nginx"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls []proxyCall
			srv := newProxyIntegrationServer(t, &calls)
			stdout, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if len(calls) != len(tc.wantCalls) {
				t.Fatalf("calls = %+v", calls)
			}
			for i, c := range calls {
				if c != tc.wantCalls[i] {
					t.Errorf("call %d = %+v, want %+v", i, c, tc.wantCalls[i])
				}
			}
			for _, w := range tc.wantOut {
				if !strings.Contains(stdout, w) {
					t.Errorf("stdout lacks %q:\n%s", w, stdout)
				}
			}
		})
	}
}

func TestProxyIntegrationCLIRejectsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"proxy", "status", "--confirm"}, &stdout, &stderr, envMap()); got == exitOK {
		t.Fatalf("status --confirm exited OK")
	}
	if !strings.Contains(stderr.String(), "proxy setup [--confirm]") {
		t.Errorf("usage missing: %s", stderr.String())
	}
}
