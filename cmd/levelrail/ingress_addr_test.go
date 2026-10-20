package main

import "testing"

// TestIngressHTTPSAddr_Default proves the env var and default fallback
// this function shares with every other simple string-env-with-default
// reader in this file (e.g. httpAddr, agentAddr).
func TestIngressHTTPSAddr_Default(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTPS_ADDR", "")
	if got := ingressHTTPSAddr(); got != defaultIngressHTTPSAddr {
		t.Errorf("ingressHTTPSAddr() = %q, want default %q", got, defaultIngressHTTPSAddr)
	}
}

func TestIngressHTTPSAddr_Override(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTPS_ADDR", ":8443")
	if got := ingressHTTPSAddr(); got != ":8443" {
		t.Errorf("ingressHTTPSAddr() = %q, want :8443", got)
	}
}

func TestIngressHTTPAddr_Default(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTP_ADDR", "")
	if got := ingressHTTPAddr(); got != defaultIngressHTTPAddr {
		t.Errorf("ingressHTTPAddr() = %q, want default %q", got, defaultIngressHTTPAddr)
	}
}

func TestIngressHTTPAddr_Override(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTP_ADDR", ":8080")
	if got := ingressHTTPAddr(); got != ":8080" {
		t.Errorf("ingressHTTPAddr() = %q, want :8080", got)
	}
}

func TestIngressPortFromAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want int
	}{
		{name: "colon-port shorthand", addr: ":8443", want: 8443},
		{name: "explicit host", addr: "127.0.0.1:8080", want: 8080},
		{name: "unparseable degrades to zero", addr: "not-an-addr", want: 0},
		{name: "empty degrades to zero", addr: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ingressPortFromAddr(tt.addr); got != tt.want {
				t.Errorf("ingressPortFromAddr(%q) = %d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}
