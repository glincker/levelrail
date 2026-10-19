package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIsHTTPS_ConfiguredTrustedProxy(t *testing.T) {
	t.Setenv(envTrustedProxies, "10.0.0.0/24")
	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{"trusted proxy reporting https", "10.0.0.5:4000", true},
		{"untrusted peer spoofing https", "10.0.1.5:4000", false},
		{"public peer spoofing https", "203.0.113.9:4000", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			r.RemoteAddr = tt.remoteAddr
			r.Header.Set("X-Forwarded-Proto", "https")
			if got := requestIsHTTPS(r); got != tt.want {
				t.Errorf("requestIsHTTPS() = %v, want %v", got, tt.want)
			}
		})
	}
}
