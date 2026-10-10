package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestIngressHTTPSURL(t *testing.T) {
	tests := []struct {
		name     string
		listen   int
		settings store.IngressSettings
		want     string
	}{
		{"unset keeps the bare host", 0, store.IngressSettings{}, "https://a.sslip.io"},
		{"default port keeps the bare host", 443, store.IngressSettings{}, "https://a.sslip.io"},
		{"listen port is part of the link", 8443, store.IngressSettings{}, "https://a.sslip.io:8443"},
		{"public 0 with ingress 8443", 8443, store.IngressSettings{PublicHTTPSPort: 0}, "https://a.sslip.io:8443"},
		{"public 443 with ingress 8443", 8443, store.IngressSettings{PublicHTTPSPort: 443}, "https://a.sslip.io"},
		{"public 8443 with ingress 8443", 8443, store.IngressSettings{PublicHTTPSPort: 8443}, "https://a.sslip.io:8443"},
		{"public 9443 with ingress 8443", 8443, store.IngressSettings{PublicHTTPSPort: 9443}, "https://a.sslip.io:9443"},
		{"upstream TLS defaults to 443", 8443, store.IngressSettings{TLSTerminatedUpstream: true}, "https://a.sslip.io"},
		{"upstream TLS with explicit port", 8443, store.IngressSettings{TLSTerminatedUpstream: true, PublicHTTPSPort: 4443}, "https://a.sslip.io:4443"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{doctorHTTPSPort: tc.listen}
			if got := rt.ingressHTTPSURL(tc.settings, "a.sslip.io"); got != tc.want {
				t.Errorf("ingressHTTPSURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHTTPSURL(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{{0, "https://h"}, {443, "https://h"}, {8443, "https://h:8443"}}
	for _, tc := range tests {
		if got := httpsURL("h", tc.port); got != tc.want {
			t.Errorf("httpsURL(%d) = %q, want %q", tc.port, got, tc.want)
		}
	}
}
