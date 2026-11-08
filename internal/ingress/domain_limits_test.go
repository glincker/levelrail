package ingress

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	for _, want := range []string{`"max_size":5242880`, `"read_timeout":"1m30s"`, `"write_timeout":"2m0s"`} {
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
