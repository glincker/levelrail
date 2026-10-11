package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

type proxyTestEnv struct {
	rt     *Router
	db     *store.DB
	dir    string
	cookie *http.Cookie
	det    proxyroutes.Detection
	probes int
}

func newProxyTestEnv(t *testing.T) *proxyTestEnv {
	t.Helper()
	rt, db := newTestRouter(t)
	env := &proxyTestEnv{rt: rt, db: db, dir: t.TempDir()}
	env.cookie = loginTestSession(t, rt, db)
	syncer := &proxyroutes.Syncer{
		Store: db, NS: "acme", Unit: "acme.service",
		Ingress:   proxyroutes.Listener{Label: "ingress", Addr: ":8088", EnvVar: "APP_INGRESS_HTTP_ADDR"},
		Dashboard: proxyroutes.Listener{Label: "dashboard", Addr: "127.0.0.1:8080", EnvVar: "APP_HTTP_ADDR"},
	}
	rt.SetProxyIntegration(syncer, db, time.Second)
	rt.publicHost = "203.0.113.10"
	rt.lookupHost = func(context.Context, string) ([]string, error) { return []string{"203.0.113.10"}, nil }
	env.det = proxyroutes.Detection{
		Kind: proxyroutes.KindTraefik, Container: "coolify-proxy", Image: "traefik:v3.6", PublishedPorts: []int{80, 443, 8080},
		DynamicDir: env.dir, EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt",
		UpstreamHost: proxyroutes.HostDockerInternal, UpstreamIP: "172.17.0.1", PublicHTTPPort: 80, PublicHTTPSPort: 443,
		APIPort: 8080, Complete: true, Missing: []string{},
	}
	rt.proxyIntegration.detect = func(context.Context) proxyroutes.Detection { return env.det }
	rt.proxyIntegration.probe = func(_ context.Context, pt proxyroutes.ProbeTarget) proxyroutes.ProbeResult {
		env.probes++
		if pt.Address != "203.0.113.10:443" {
			return proxyroutes.ProbeResult{Err: "wrong address " + pt.Address}
		}
		return proxyroutes.ProbeResult{Reachable: true, StatusCode: 200, CertIssuer: "R11", CertTrusted: true, CertNotAfter: time.Now().Add(80 * 24 * time.Hour)}
	}
	rt.proxyIntegration.routerLoaded = func(context.Context, string, string, time.Duration) (bool, error) { return true, nil }
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 3000, Domains: []string{"app.example.com"}}); err != nil {
		t.Fatal(err)
	}
	return env
}

func (e *proxyTestEnv) do(t *testing.T, method, path, body string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, method, path, body))
	return rec.Code, rec.Body.Bytes()
}

func TestProxyIntegrationSetup(t *testing.T) {
	env := newProxyTestEnv(t)
	file := filepath.Join(env.dir, "acme-managed-app.example.com.yaml")

	code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{}`)
	var dry proxySetupResponse
	if err := json.Unmarshal(body, &dry); err != nil || code != http.StatusOK || !dry.DryRun {
		t.Fatalf("dry run: %d %s", code, body)
	}
	if len(dry.Changes) == 0 || dry.Settings.Integration != store.ProxyIntegrationOff {
		t.Errorf("dry run changes = %v settings = %+v", dry.Changes, dry.Settings)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote a file: %v", err)
	}

	for i := 0; i < 2; i++ {
		code, body = env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`)
		var res proxySetupResponse
		if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK || res.DryRun {
			t.Fatalf("confirm %d: %d %s", i, code, body)
		}
		if i == 1 && len(res.Changes) != 0 {
			t.Errorf("second setup is not idempotent: %v", res.Changes)
		}
		if !res.Ingress.TLSTerminatedUpstream || res.Ingress.PublicHTTPSPort != 443 || res.Ingress.HTTPPort != 8088 {
			t.Errorf("ingress = %+v", res.Ingress)
		}
		if len(res.Domains) != 1 {
			t.Fatalf("domains = %+v", res.Domains)
		}
		d := res.Domains[0]
		if d.State != proxyroutes.StateWritten || !d.Reachable || d.ProxyLoaded == nil || !*d.ProxyLoaded || d.Certificate == nil || !d.Certificate.Valid || d.Target != proxyroutes.TargetApp || d.App != "web" {
			t.Errorf("domain = %+v", d)
		}
		for _, s := range res.Steps {
			if s.State != proxyStepDone {
				t.Errorf("step %s = %s (%s)", s.ID, s.State, s.Detail)
			}
		}
	}
	got, err := os.ReadFile(file) //nolint:gosec // test temp dir
	if err != nil || !strings.HasPrefix(string(got), "# managed by acme\n") || !strings.Contains(string(got), "url: http://host.docker.internal:8088") {
		t.Fatalf("route file = %q, %v", got, err)
	}

	code, body = env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/verify?domain=nope.example.com", "")
	if code != http.StatusNotFound {
		t.Errorf("verify unknown domain = %d %s", code, body)
	}
	code, _ = env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/verify?domain=app.example.com", "")
	if code != http.StatusOK || env.probes != 3 {
		t.Errorf("verify = %d probes = %d", code, env.probes)
	}

	code, body = env.do(t, http.MethodPut, "/api/v1/settings/proxy-integration", `{"integration":"off","dynamic_dir":"`+env.dir+`"}`)
	if code != http.StatusOK {
		t.Fatalf("disable = %d %s", code, body)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("disable left the route file: %v", err)
	}
	if code, _ = env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/apply", ""); code != http.StatusConflict {
		t.Errorf("apply while off = %d", code)
	}
}

func TestProxyIntegrationSetupConflicts(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*proxyTestEnv)
		body     string
		wantCode string
		wantText string
	}{
		{"nothing detected", func(e *proxyTestEnv) { e.det = proxyroutes.None() }, `{"confirm":true}`, proxyCodeNotDetected, "no reverse proxy"},
		{"nginx", func(e *proxyTestEnv) { e.det.Kind = proxyroutes.KindNginx }, `{"confirm":true}`, proxyCodeUnsupported, "Traefik only"},
		{"incomplete", func(e *proxyTestEnv) {
			e.det.Complete, e.det.Missing = false, []string{"no certificate resolver"}
		}, `{"confirm":true}`, proxyCodeIncomplete, "no certificate resolver"},
		{"loopback ingress", func(e *proxyTestEnv) { e.rt.proxyIntegration.syncer.Ingress.Addr = "127.0.0.1:8088" }, `{}`, proxyroutes.UpstreamUnreachableCode, "cannot reach"},
		{"directory missing", func(e *proxyTestEnv) { e.det.DynamicDir = filepath.Join(e.dir, "nope") }, `{}`, proxyCodeDirInvalid, "does not exist"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newProxyTestEnv(t)
			tc.mutate(env)
			code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", tc.body)
			var c proxyConflict
			_ = json.Unmarshal(body, &c)
			if code != http.StatusConflict || c.Code != tc.wantCode || !strings.Contains(c.Error, tc.wantText) {
				t.Errorf("got %d %s", code, body)
			}
		})
	}
}

func TestProxyIntegrationSetupDirOverride(t *testing.T) {
	env := newProxyTestEnv(t)
	env.det.DynamicDir = ""
	env.det.Complete = false
	code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"dynamic_dir":"relative"}`)
	if code != http.StatusBadRequest {
		t.Errorf("relative dir = %d %s", code, body)
	}
}

func TestProxyIntegrationAbilities(t *testing.T) {
	env := newProxyTestEnv(t)
	const plaintext = "proxy-read-only-token" //nolint:gosec // fake fixture, not a real credential
	if err := env.db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_proxy_ro", Name: "read only", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/v1/system/proxy-integration", http.StatusOK},
		{http.MethodPost, "/api/v1/system/proxy-integration/setup", http.StatusForbidden},
		{http.MethodPost, "/api/v1/system/proxy-integration/apply", http.StatusForbidden},
		{http.MethodPost, "/api/v1/system/proxy-integration/verify", http.StatusForbidden},
		{http.MethodPut, "/api/v1/settings/proxy-integration", http.StatusForbidden},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		env.rt.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestProxyIntegrationGetContract(t *testing.T) {
	env := newProxyTestEnv(t)
	code, body := env.do(t, http.MethodGet, "/api/v1/system/proxy-integration", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d %s", code, body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"detected", "settings", "ingress", "domains", "steps"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing %q in %s", k, body)
		}
	}
	for _, k := range []string{`"published_ports"`, `"complete"`, `"missing"`, `"integration":"off"`, `"dashboard_addr"`, `"proxy_loaded":null`, `"certificate":null`, `"state":"missing"`} {
		if !strings.Contains(string(body), k) {
			t.Errorf("body lacks %s: %s", k, body)
		}
	}
}

func TestProxyIntegrationSettingsValidation(t *testing.T) {
	env := newProxyTestEnv(t)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"unknown mode", `{"integration":"nginx"}`, http.StatusBadRequest},
		{"relative dir", `{"integration":"off","dynamic_dir":"x"}`, http.StatusBadRequest},
		{"on without dir", `{"integration":"traefik_file"}`, http.StatusBadRequest},
		{"bad entrypoint", `{"integration":"traefik_file","dynamic_dir":"` + env.dir + `","entrypoint_http":"a b","entrypoint_https":"https","upstream_host":"10.0.0.1"}`, http.StatusBadRequest},
		{"bad upstream", `{"integration":"traefik_file","dynamic_dir":"` + env.dir + `","entrypoint_http":"http","entrypoint_https":"https","upstream_host":"*.x"}`, http.StatusBadRequest},
		{"valid", `{"integration":"traefik_file","dynamic_dir":"` + env.dir + `","entrypoint_http":"http","entrypoint_https":"https","upstream_host":"10.0.0.1"}`, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code, body := env.do(t, http.MethodPut, "/api/v1/settings/proxy-integration", tc.body); code != tc.want {
				t.Errorf("got %d %s", code, body)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(env.dir, "acme-managed-app.example.com.yaml")); err != nil {
		t.Errorf("valid settings did not write the route: %v", err)
	}
}
