package proxycoexist

import (
	"strings"
	"testing"
)

func TestBuildTarget(t *testing.T) {
	tests := []struct {
		name       string
		target     Target
		wantRebind string
		wantInSnip []string
	}{
		{
			name:       "app domain to a loopback ingress",
			target:     Target{Domain: "app.example.com", Kind: Traefik, ListenAddr: "127.0.0.1:8088", Gateway: "172.18.0.1", Name: "acme-web", EnvVar: "APP_INGRESS_HTTP_ADDR"},
			wantRebind: "APP_INGRESS_HTTP_ADDR=172.18.0.1:8088",
			wantInSnip: []string{"acme-web:", "acme-web-http:", "middlewares: [acme-web-https]", "url: http://172.18.0.1:8088"},
		},
		{
			name:       "app domain to a wildcard ingress",
			target:     Target{Domain: "app.example.com", Kind: Traefik, ListenAddr: ":8088", Gateway: "172.18.0.1", Name: "acme-web", EnvVar: "APP_INGRESS_HTTP_ADDR"},
			wantInSnip: []string{"url: http://172.18.0.1:8088"},
		},
		{
			name:       "nginx",
			target:     Target{Domain: "app.example.com", Kind: Nginx, ListenAddr: ":8088", Name: "acme-web", EnvVar: "APP_INGRESS_HTTP_ADDR"},
			wantInSnip: []string{"proxy_pass http://127.0.0.1:8088"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := BuildTarget(tc.target)
			if err != nil {
				t.Fatal(err)
			}
			if p.Rebind != tc.wantRebind {
				t.Errorf("rebind = %q, want %q", p.Rebind, tc.wantRebind)
			}
			for _, s := range tc.wantInSnip {
				if !strings.Contains(p.Snippet, s) {
					t.Errorf("snippet lacks %q:\n%s", s, p.Snippet)
				}
			}
		})
	}
}
