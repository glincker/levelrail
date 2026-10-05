package ingress

import (
	"strings"
	"testing"
	"time"
)

func TestHardeningFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, h Hardening)
	}{
		{name: "defaults", check: func(t *testing.T, h Hardening) {
			if h.ReadHeaderTimeout != 10*time.Second || h.MaxBodyBytes != 1<<30 || len(h.Protocols) != 3 {
				t.Errorf("unexpected defaults: %+v", h)
			}
		}},
		{name: "overrides", env: map[string]string{EnvReadHeaderTimeout: "3s", EnvMaxBodyBytes: "0", EnvProtocols: "h1,h2", EnvClientIPHeaders: "CF-Connecting-IP, X-Forwarded-For"}, check: func(t *testing.T, h Hardening) {
			if h.ReadHeaderTimeout != 3*time.Second || h.MaxBodyBytes != 0 || len(h.Protocols) != 2 || len(h.ClientIPHeaders) != 2 {
				t.Errorf("unexpected: %+v", h)
			}
		}},
		{name: "private ranges expand", env: map[string]string{EnvTrustedProxies: "private_ranges,203.0.113.0/24"}, check: func(t *testing.T, h Hardening) {
			if len(h.TrustedProxies) != len(privateRanges)+1 {
				t.Errorf("trusted proxies = %v", h.TrustedProxies)
			}
		}},
		{name: "bad duration", env: map[string]string{EnvIdleTimeout: "soon"}, wantErr: EnvIdleTimeout},
		{name: "negative bytes", env: map[string]string{EnvMaxHeaderBytes: "-1"}, wantErr: EnvMaxHeaderBytes},
		{name: "bad protocol", env: map[string]string{EnvProtocols: "h4"}, wantErr: "unknown protocol"},
		{name: "bad proxy", env: map[string]string{EnvTrustedProxies: "not-an-ip"}, wantErr: "not an IP"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := HardeningFromEnv(func(k string) string { return tt.env[k] })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, h)
		})
	}
}

func TestHardeningApplyProxy(t *testing.T) {
	h := DefaultHardening()
	single := NewReverseProxyHandler("127.0.0.1:1")
	h.applyProxy(&single)
	if single.HealthChecks != nil {
		t.Error("single upstream must not get passive ejection: it would extend an outage after recovery")
	}
	if single.LoadBalancing == nil || single.LoadBalancing.TryDuration != "2s" {
		t.Errorf("retry window not set: %+v", single.LoadBalancing)
	}
	if single.Headers.Request.Set["X-Real-IP"][0] != "{http.vars.client_ip}" {
		t.Error("X-Real-IP not forwarded")
	}

	pool := NewLBReverseProxyHandler(&LBRoute{Upstreams: []string{"a:1", "b:1"}})
	h.applyProxy(&pool)
	if pool.HealthChecks == nil || pool.HealthChecks.Passive == nil || pool.HealthChecks.Passive.MaxFails != 1 {
		t.Errorf("pool should eject a failed replica: %+v", pool.HealthChecks)
	}

	custom := NewLBReverseProxyHandler(&LBRoute{Upstreams: []string{"a:1", "b:1"}, TryDuration: "9s", PassiveHealth: &PassiveHealthChecks{MaxFails: 4}})
	h.applyProxy(&custom)
	if custom.LoadBalancing.TryDuration != "9s" || custom.HealthChecks.Passive.MaxFails != 4 {
		t.Error("operator values must win over defaults")
	}
}

func TestHardeningApplyServer(t *testing.T) {
	h := DefaultHardening()
	h.TrustedProxies = []string{"10.0.0.0/8"}
	h.ClientIPHeaders = []string{"CF-Connecting-IP"}
	var s Server
	h.applyServer(&s)
	if s.ReadHeaderTimeout != "10s" || s.MaxHeaderBytes != 128<<10 || s.ReadTimeout != "" {
		t.Errorf("server limits wrong: %+v", s)
	}
	if s.TrustedProxies == nil || s.TrustedProxiesStrict != 1 {
		t.Errorf("trusted proxies wrong: %+v", s)
	}
	if s.Errors == nil || len(s.Errors.Routes) != 2 {
		t.Error("friendly error routes missing")
	}
}
