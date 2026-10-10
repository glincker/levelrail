package api

import "testing"

func TestIngressHTTPSURL(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{"unset keeps the bare host", 0, "https://a.sslip.io"},
		{"default port keeps the bare host", 443, "https://a.sslip.io"},
		{"non-default port is part of the link", 8443, "https://a.sslip.io:8443"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{doctorHTTPSPort: tc.port}
			if got := rt.ingressHTTPSURL("a.sslip.io"); got != tc.want {
				t.Errorf("ingressHTTPSURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
