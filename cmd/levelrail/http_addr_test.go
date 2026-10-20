package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestResolveHTTPAddr(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		inContainer bool
		want        string
	}{
		{"explicit address wins", map[string]string{"APP_HTTP_ADDR": "0.0.0.0:9000", "APP_DASHBOARD_URL": "https://lr.example.com"}, false, "0.0.0.0:9000"},
		{"bare binary without a dashboard URL keeps all interfaces", nil, false, ":8080"},
		{"https dashboard URL fronted by ingress binds loopback", map[string]string{"APP_DASHBOARD_URL": "https://lr.example.com"}, false, "127.0.0.1:8080"},
		{"plain http dashboard URL is not fronted by TLS", map[string]string{"APP_DASHBOARD_URL": "http://10.0.0.5:8080"}, false, ":8080"},
		{"container image keeps all interfaces", map[string]string{"APP_DASHBOARD_URL": "https://lr.example.com"}, true, ":8080"},
		{"container without a dashboard URL keeps all interfaces", nil, true, ":8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveHTTPAddr(func(k string) string { return tt.env[k] }, tt.inContainer)
			if got != tt.want {
				t.Errorf("resolveHTTPAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHTTPAddrExposed(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{":8080", true},
		{"0.0.0.0:8080", true},
		{"[::]:8080", true},
		{"10.0.0.5:8080", true},
		{"lr.internal:8080", true},
		{"127.0.0.1:8080", false},
		{"[::1]:8080", false},
		{"localhost:8080", false},
		{"not-an-address", false},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			if got := httpAddrExposed(tt.addr); got != tt.want {
				t.Errorf("httpAddrExposed(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestWarnExposedHTTPAddr(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		inContainer bool
		wantWarn    bool
	}{
		{"exposed bare binary", ":8080", false, true},
		{"loopback", "127.0.0.1:8080", false, false},
		{"container published port is the boundary", ":8080", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			var w httpAddrWarner
			w.warn(logger, tt.addr, tt.inContainer)
			w.warn(logger, tt.addr, tt.inContainer)
			got := strings.Count(buf.String(), "level=WARN")
			want := 0
			if tt.wantWarn {
				want = 1
			}
			if got != want {
				t.Errorf("warnings = %d, want %d (one-time): %s", got, want, buf.String())
			}
			if tt.wantWarn && !strings.Contains(buf.String(), "APP_HTTP_ADDR") {
				t.Errorf("warning must name the override: %s", buf.String())
			}
		})
	}
}
