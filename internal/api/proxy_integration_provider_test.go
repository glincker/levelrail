package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeProxyExec struct {
	calls   []string
	failOn  string
	message string
}

func (f *fakeProxyExec) Exec(_ context.Context, _ string, cmd []string) (string, error) {
	line := strings.Join(cmd, " ")
	f.calls = append(f.calls, line)
	if line == f.failOn {
		return "", &proxyroutes.ExecError{Output: f.message, Err: errors.New("exited 1")}
	}
	return "", nil
}

func nginxEnv(t *testing.T) (*proxyTestEnv, *fakeProxyExec) {
	t.Helper()
	env := newProxyTestEnv(t)
	ex := &fakeProxyExec{}
	env.rt.proxyIntegration.syncer.Exec = ex
	env.det.Kind, env.det.Image = proxyroutes.KindNginx, "nginx:1.27"
	env.det.EntrypointHTTP, env.det.EntrypointHTTPS, env.det.CertResolver, env.det.APIPort = "", "", "", 0
	env.det.CertDir = "/etc/letsencrypt/live"
	return env, ex
}

func TestProxyIntegrationNginxSetup(t *testing.T) {
	env, ex := nginxEnv(t)
	file := filepath.Join(env.dir, "acme-managed-app.example.com.conf")

	code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"provider":"nginx"}`)
	var dry proxySetupResponse
	if err := json.Unmarshal(body, &dry); err != nil || code != http.StatusOK || !dry.DryRun {
		t.Fatalf("dry run: %d %s", code, body)
	}
	if !contains(dry.Changes, "write acme-managed-app.example.com.conf") || len(ex.calls) != 0 {
		t.Errorf("dry run changes = %v, exec calls = %v", dry.Changes, ex.calls)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote a file: %v", err)
	}

	code, body = env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`)
	var res proxySetupResponse
	if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK || res.DryRun {
		t.Fatalf("confirm: %d %s", code, body)
	}
	if res.Provider != proxyroutes.KindNginx || res.Settings.Integration != store.ProxyIntegrationNginxFile || res.Settings.Container != "coolify-proxy" {
		t.Errorf("provider = %q settings = %+v", res.Provider, res.Settings)
	}
	if !reflect.DeepEqual(ex.calls, []string{"nginx -t", "nginx -s reload"}) {
		t.Errorf("exec calls = %v", ex.calls)
	}
	if got, err := os.ReadFile(file); err != nil || !strings.Contains(string(got), "proxy_pass http://host.docker.internal:8088;") { //nolint:gosec // test temp dir
		t.Errorf("route file = %q, %v", got, err)
	}
}

func TestProxyIntegrationNginxRejectedIsRolledBack(t *testing.T) {
	env, ex := nginxEnv(t)
	ex.failOn = "nginx -t"
	ex.message = `nginx: [emerg] cannot load certificate "/etc/letsencrypt/live/app.example.com/fullchain.pem"`

	code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`)
	var c proxyConflict
	_ = json.Unmarshal(body, &c)
	if code != http.StatusConflict || c.Code != proxyCodeRejected || !strings.Contains(c.Error, ex.message) {
		t.Fatalf("got %d %s", code, body)
	}
	if entries, _ := os.ReadDir(env.dir); len(entries) != 0 {
		t.Errorf("files left after a rejected apply: %v", entries)
	}
	s, err := env.db.GetProxyIntegrationSettings(context.Background())
	if err != nil || s.Mode != store.ProxyIntegrationOff {
		t.Errorf("settings = %+v, %v", s, err)
	}
	ing, err := env.rt.ingressSettings.GetIngressSettings(context.Background())
	if err != nil || ing.TLSTerminatedUpstream {
		t.Errorf("tls upstream switched on by a rejected apply: %+v, %v", ing, err)
	}
}

func TestProxyIntegrationManualProviderOnlyRendersASnippet(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"caddy", "reverse_proxy host.docker.internal:8088"},
		{"haproxy", "server upstream host.docker.internal:8088"},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			env := newProxyTestEnv(t)
			env.det.Kind = tc.provider
			code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`)
			var res proxySetupResponse
			if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK {
				t.Fatalf("got %d %s", code, body)
			}
			if !res.Manual || !res.DryRun || !strings.Contains(res.Snippet, tc.want) || res.Settings.Integration != store.ProxyIntegrationOff {
				t.Errorf("response = %+v", res)
			}
			if entries, _ := os.ReadDir(env.dir); len(entries) != 0 {
				t.Errorf("manual provider wrote %v", entries)
			}
		})
	}
}

func TestProxyIntegrationListsProviders(t *testing.T) {
	env := newProxyTestEnv(t)
	code, body := env.do(t, http.MethodGet, "/api/v1/system/proxy-integration", "")
	var res proxyIntegrationResource
	if err := json.Unmarshal(body, &res); err != nil || code != http.StatusOK {
		t.Fatalf("got %d %s", code, body)
	}
	modes := map[string]string{}
	for _, p := range res.Providers {
		modes[p.Kind] = p.Mode
	}
	want := map[string]string{"embedded": "builtin", "traefik": "managed", "nginx": "managed", "caddy": "manual", "haproxy": "manual"}
	if !reflect.DeepEqual(modes, want) || res.Provider != proxyProviderEmbedded {
		t.Errorf("providers = %+v selected %q", res.Providers, res.Provider)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
