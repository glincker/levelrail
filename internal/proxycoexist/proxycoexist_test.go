package proxycoexist

import (
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		listen      string
		gateway     string
		wantUpstrm  string
		wantRebind  bool
		wantInSnip  string
		wantErrText string
	}{
		{"container proxy, loopback dashboard needs a rebind", Traefik, "127.0.0.1:18080", "172.18.0.1", "http://172.18.0.1:18080", true, "url: http://172.18.0.1:18080", ""},
		{"container proxy, already on the gateway", Traefik, "172.18.0.1:18080", "172.18.0.1", "http://172.18.0.1:18080", false, "Host(`console.example.com`)", ""},
		{"host proxy reaches loopback directly", Nginx, "127.0.0.1:8080", "", "http://127.0.0.1:8080", false, "proxy_pass http://127.0.0.1:8080", ""},
		{"caddy snippet", Caddy, "127.0.0.1:8080", "", "http://127.0.0.1:8080", false, "reverse_proxy 127.0.0.1:8080", ""},
		{"bad domain", Caddy, "127.0.0.1:8080", "", "", false, "", "hostname"},
		{"bad listen address", Caddy, "nonsense", "", "", false, "", "listen address"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			domain := "console.example.com"
			if tc.wantErrText == "hostname" {
				domain = "not a host"
			}
			p, err := Build(domain, tc.kind, tc.listen, tc.gateway)
			if tc.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrText) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if p.UpstreamURL != tc.wantUpstrm {
				t.Errorf("upstream = %q, want %q", p.UpstreamURL, tc.wantUpstrm)
			}
			if p.NeedsRebind != tc.wantRebind {
				t.Errorf("needs rebind = %v, want %v", p.NeedsRebind, tc.wantRebind)
			}
			if !strings.Contains(p.Snippet, tc.wantInSnip) {
				t.Errorf("snippet missing %q:\n%s", tc.wantInSnip, p.Snippet)
			}
			if len(p.Steps) == 0 {
				t.Error("no steps")
			}
		})
	}
}

func TestKindFromImage(t *testing.T) {
	for image, want := range map[string]Kind{"traefik:v3.6": Traefik, "nginx:1.27": Nginx, "caddy:2": Caddy, "lucaslorentz/caddy-docker-proxy": Caddy} {
		if got, ok := KindFromImage(image); !ok || got != want {
			t.Errorf("KindFromImage(%q) = %q, %v, want %q", image, got, ok, want)
		}
	}
	if _, ok := KindFromImage("postgres:17"); ok {
		t.Error("postgres detected as a proxy")
	}
}
