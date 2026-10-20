package ingress

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSSLIPHost(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"134.209.118.96", "134-209-118-96.sslip.io", true},
		{" 203.0.113.5 ", "203-0-113-5.sslip.io", true},
		{"10.0.0.4", "", false},
		{"example.com", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, ok := SSLIPHost(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("SSLIPHost(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestResolvePublicHost(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("203.0.113.9\n")) }))
	defer good.Close()
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("192.168.1.2")) }))
	defer private.Close()

	tests := []struct {
		name       string
		env        map[string]string
		wantHost   string
		wantSource string
	}{
		{"env override wins", map[string]string{"APP_PUBLIC_HOST": "198.51.100.7", "APP_PUBLIC_IP_PROBE_URLS": good.URL}, "198.51.100.7", PublicHostSourceEnv},
		{"detect off", map[string]string{"APP_PUBLIC_IP_DETECT": "off", "APP_PUBLIC_IP_PROBE_URLS": good.URL}, "", PublicHostSourceDisabled},
		{"detected", map[string]string{"APP_PUBLIC_IP_PROBE_URLS": good.URL}, "203.0.113.9", PublicHostSourceDetected},
		{"private answer rejected", map[string]string{"APP_PUBLIC_IP_PROBE_URLS": private.URL}, "", PublicHostSourceNone},
		{"one good among bad", map[string]string{"APP_PUBLIC_IP_PROBE_URLS": private.URL + "," + good.URL}, "203.0.113.9", PublicHostSourceDetected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_PUBLIC_HOST", "")
			t.Setenv("APP_PUBLIC_IP_DETECT", "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			host, source := ResolvePublicHost(context.Background())
			if host != tc.wantHost || source != tc.wantSource {
				t.Fatalf("got %q/%q, want %q/%q", host, source, tc.wantHost, tc.wantSource)
			}
		})
	}
}

func TestNewProbeClient_IPv4OnlyRefusesIPv6(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback on this host")
	}
	srv.Listener = l
	srv.Start()
	defer srv.Close()
	if ip := fetchPublicIP(context.Background(), newProbeClient("tcp4"), srv.URL); ip != "" {
		t.Errorf("tcp4 client reached an IPv6-only server: %q", ip)
	}
}
