package proxyroutes

import (
	"strings"
	"testing"
)

const testNS Namespace = "acme"

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"glinrv2.glinr.com", "glinrv2.glinr.com", false},
		{"App.Example.COM", "app.example.com", false},
		{"xn--bcher-kva.example", "xn--bcher-kva.example", false},
		{"", "", true},
		{"*.example.com", "", true},
		{"example", "", true},
		{"a/b.example.com", "", true},
		{"../etc.example.com", "", true},
		{"a.example.com:443", "", true},
		{" a.example.com", "", true},
		{"a..example.com", "", true},
		{"-a.example.com", "", true},
		{"a_b.example.com", "", true},
		{"a`.example.com", "", true},
	}
	for _, tc := range tests {
		got, err := ValidateDomain(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ValidateDomain(%q) = %q, %v; want %q, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestFileName_RejectsAnythingThatSanitisesDifferently(t *testing.T) {
	if got, err := testNS.FileName("a.example.com"); err != nil || got != "acme-managed-a.example.com.yaml" {
		t.Fatalf("FileName() = %q, %v", got, err)
	}
	for _, in := range []string{"A.example.com", "a.example.com/x", "*.example.com"} {
		if _, err := testNS.FileName(in); err == nil {
			t.Errorf("FileName(%q): expected an error", in)
		}
	}
}

func TestRender(t *testing.T) {
	base := Route{Domain: "glinrv2.glinr.com", EntrypointHTTP: "http", EntrypointHTTPS: "https", CertResolver: "letsencrypt", Upstream: "host.docker.internal:8088"}
	tests := []struct {
		name     string
		mutate   func(*Route)
		contains []string
		absent   []string
		wantErr  bool
	}{
		{name: "coolify defaults", mutate: func(*Route) {}, contains: []string{
			"# managed by acme\n",
			"acme-managed-glinrv2_glinr_com:\n      rule: Host(`glinrv2.glinr.com`)\n      entryPoints: [https]",
			"certResolver: letsencrypt",
			"entryPoints: [http]\n      middlewares: [acme-managed-glinrv2_glinr_com-https]",
			"redirectScheme:\n        scheme: https",
			"- url: http://host.docker.internal:8088",
		}},
		{name: "custom entrypoints", mutate: func(r *Route) { r.EntrypointHTTP, r.EntrypointHTTPS = "web", "websecure" }, contains: []string{"entryPoints: [websecure]", "entryPoints: [web]"}},
		{name: "no resolver", mutate: func(r *Route) { r.CertResolver = "" }, contains: []string{"tls: {}"}, absent: []string{"certResolver"}},
		{name: "ip upstream", mutate: func(r *Route) { r.Upstream = "10.0.1.1:8080" }, contains: []string{"url: http://10.0.1.1:8080"}},
		{name: "ipv6 upstream", mutate: func(r *Route) { r.Upstream = "[fd00::1]:8088" }, contains: []string{"url: http://[fd00::1]:8088"}},
		{name: "wildcard", mutate: func(r *Route) { r.Domain = "*.glinr.com" }, wantErr: true},
		{name: "uppercase", mutate: func(r *Route) { r.Domain = "Glinr.com" }, wantErr: true},
		{name: "yaml injection entrypoint", mutate: func(r *Route) { r.EntrypointHTTP = "http]\n  x: [y" }, wantErr: true},
		{name: "bad resolver", mutate: func(r *Route) { r.CertResolver = "le resolver" }, wantErr: true},
		{name: "upstream without port", mutate: func(r *Route) { r.Upstream = "host.docker.internal" }, wantErr: true},
		{name: "upstream with path", mutate: func(r *Route) { r.Upstream = "evil.com/x:80" }, wantErr: true},
		{name: "upstream port zero", mutate: func(r *Route) { r.Upstream = "127.0.0.1:0" }, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.mutate(&r)
			got, err := testNS.Render(r)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got:\n%s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			s := string(got)
			if !strings.HasPrefix(s, testNS.Header()+"\n") {
				t.Errorf("missing header:\n%s", s)
			}
			for _, c := range tc.contains {
				if !strings.Contains(s, c) {
					t.Errorf("missing %q in:\n%s", c, s)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(s, a) {
					t.Errorf("unexpected %q in:\n%s", a, s)
				}
			}
		})
	}
}

func TestRouterName(t *testing.T) {
	if got := testNS.RouterName("a.b.com"); got != "acme-managed-a_b_com@file" {
		t.Errorf("RouterName() = %q", got)
	}
}
