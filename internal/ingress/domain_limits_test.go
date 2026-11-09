package ingress

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func renderLimitsRoute(t *testing.T, r ProxyRoute, h *Hardening) string {
	t.Helper()
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "srv", ListenAddr: ":443", Hardening: h, Routes: []ProxyRoute{r}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(cfg.Apps.HTTP.Servers["srv"].Routes)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDomainLimits_Rendering(t *testing.T) {
	base := ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "127.0.0.1:9000"}

	unset := renderLimitsRoute(t, base, nil)
	for _, key := range []string{"read_timeout", "write_timeout", "request_body", "transport"} {
		if strings.Contains(unset, key) {
			t.Errorf("route without domain limits renders %q: %s", key, unset)
		}
	}

	set := base
	set.MaxBodyBytes, set.ReadTimeout, set.WriteTimeout = 5<<20, 90*time.Second, 2*time.Minute
	got := renderLimitsRoute(t, set, nil)
	for _, want := range []string{`"max_size":5242880`, `"handler":"levelrail_body_guard"`, `"read_timeout":"1m30s"`, `"write_timeout":"2m0s"`} {
		if !strings.Contains(got, want) {
			t.Errorf("route with domain limits missing %s: %s", want, got)
		}
	}

	h := DefaultHardening()
	withHardening := renderLimitsRoute(t, set, &h)
	if !strings.Contains(withHardening, `"dial_timeout":"2s"`) || !strings.Contains(withHardening, `"read_timeout":"1m30s"`) {
		t.Errorf("hardening dial timeout and domain read timeout should both render: %s", withHardening)
	}
}

func TestBodyGuard_RejectsOnContentLength(t *testing.T) {
	m := &bodyGuardModule{MaxSize: 10}
	var reached bool
	next := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { reached = true; return nil })

	big := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789ab"))
	err := m.ServeHTTP(httptest.NewRecorder(), big, next)
	var he caddyhttp.HandlerError
	if !errors.As(err, &he) || he.StatusCode != http.StatusRequestEntityTooLarge || reached {
		t.Fatalf("over limit: err=%v reached=%v", err, reached)
	}

	ok := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("0123456789"))
	if err := m.ServeHTTP(httptest.NewRecorder(), ok, next); err != nil || !reached {
		t.Fatalf("at limit: err=%v reached=%v", err, reached)
	}
}
