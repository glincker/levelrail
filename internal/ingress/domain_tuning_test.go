package ingress

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDomainTuning_Rendering(t *testing.T) {
	base := ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "127.0.0.1:9000", BasicAuth: &BasicAuthAccount{Username: "u", Password: "p"}}
	plain := renderLimitsRoute(t, base, nil)
	for _, key := range []string{`"encode"`, "flush_interval", "stream_timeout", "dial_timeout"} {
		if strings.Contains(plain, key) {
			t.Errorf("untuned route renders %q: %s", key, plain)
		}
	}

	api := base
	api.Tuning = &DomainTuning{Compression: true, DialTimeout: 5 * time.Second, ResponseHeaderTimeout: time.Minute}
	got := renderLimitsRoute(t, api, nil)
	for _, want := range []string{`"handler":"encode"`, `"dial_timeout":"5s"`, `"response_header_timeout":"1m0s"`} {
		if !strings.Contains(got, want) {
			t.Errorf("api tuning missing %s: %s", want, got)
		}
	}
	if strings.Index(got, `"encode"`) > strings.Index(got, `"reverse_proxy"`) {
		t.Errorf("encode must sit before the reverse proxy: %s", got)
	}

	ws := base
	ws.Tuning = &DomainTuning{Streaming: true, StreamTimeout: time.Hour, StreamCloseDelay: time.Minute}
	got = renderLimitsRoute(t, ws, nil)
	for _, want := range []string{`"flush_interval":-1`, `"stream_timeout":"1h0m0s"`, `"stream_close_delay":"1m0s"`} {
		if !strings.Contains(got, want) {
			t.Errorf("websocket tuning missing %s: %s", want, got)
		}
	}
	// Tuning must leave host matching and the auth handler exactly as they were.
	for _, want := range []string{`"host":["a.test"]`, `"authentication"`} {
		if strings.Contains(plain, want) != strings.Contains(got, want) {
			t.Errorf("tuning changed %s: plain=%s tuned=%s", want, plain, got)
		}
	}
	if strings.Index(got, `"authentication"`) > strings.Index(got, `"reverse_proxy"`) {
		t.Errorf("auth must still run before the proxy: %s", got)
	}
}

// Caddy itself must accept every tuning knob, so a tuned domain never relies on the fallback.
func TestDomainTuning_CaddyValidates(t *testing.T) {
	r := ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "127.0.0.1:9000", Tuning: &DomainTuning{
		Compression: true, Streaming: true, DialTimeout: time.Second, ResponseHeaderTimeout: time.Second,
		StreamTimeout: time.Hour, StreamCloseDelay: time.Minute,
	}}
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "srv", ListenAddr: "127.0.0.1:0", Routes: []ProxyRoute{r}})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConfig(payload); err != nil {
		t.Fatalf("caddy rejected tuned config: %v", err)
	}
}
