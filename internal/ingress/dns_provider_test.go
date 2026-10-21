package ingress

import (
	"net"
	"testing"
)

func TestFallbackDomainFor(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		app    string
		choice ProviderChoice
		want   string
	}{
		{"default is sslip", "203.0.113.5", "web", ProviderChoice{}, "web.203-0-113-5.sslip.io"},
		{"nip", "203.0.113.5", "web", ProviderChoice{Provider: "nip.io"}, "web.203-0-113-5.nip.io"},
		{"traefik", "203.0.113.5", "My App", ProviderChoice{Provider: "traefik.me"}, "my-app.203-0-113-5.traefik.me"},
		{"traefik is ipv4 only", "2001:db8::1", "web", ProviderChoice{Provider: "traefik.me"}, ""},
		{"sslip ipv6", "2606:4700::1", "web", ProviderChoice{Provider: "sslip.io"}, "web.2606-4700--1.sslip.io"},
		{"custom base", "203.0.113.5", "web", ProviderChoice{Provider: "custom", CustomBase: "Apps.Example.com"}, "web.apps.example.com"},
		{"custom works with a hostname public host", "box.example.net", "web", ProviderChoice{Provider: "custom", CustomBase: "apps.example.com"}, "web.apps.example.com"},
		{"custom public suffix rejected", "203.0.113.5", "web", ProviderChoice{Provider: "custom", CustomBase: "co.uk"}, ""},
		{"custom ip rejected", "203.0.113.5", "web", ProviderChoice{Provider: "custom", CustomBase: "203.0.113.5"}, ""},
		{"unknown provider", "203.0.113.5", "web", ProviderChoice{Provider: "nope.io"}, ""},
		{"private ip has no free hostname", "10.0.0.5", "web", ProviderChoice{}, ""},
		{"local mode ignores provider", "localhost", "web", ProviderChoice{Provider: "nip.io"}, "web.localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FallbackDomainFor(tt.host, tt.app, tt.choice)
			if got != tt.want || ok != (tt.want != "") {
				t.Fatalf("got %q ok=%v, want %q", got, ok, tt.want)
			}
		})
	}
}

func TestEncodeIPFormats(t *testing.T) {
	v4 := net.ParseIP("203.0.113.5")
	if got := encodeIP(v4, IPFormatDashed); got != "203-0-113-5" {
		t.Errorf("dashed = %q", got)
	}
	if got := encodeIP(v4, IPFormatDotted); got != "203.0.113.5" {
		t.Errorf("dotted = %q", got)
	}
	p := DNSProvider{Suffix: "dots.test", IPFormat: IPFormatDotted}
	if got, _ := providerHost(p, "203.0.113.5", "web"); got != "web.203.0.113.5.dots.test" {
		t.Errorf("dotted provider host = %q", got)
	}
}

func TestValidateCustomBase(t *testing.T) {
	for base, wantErr := range map[string]bool{
		"apps.example.com": false, "example.com": false, "com": true, "co.uk": true, "": true,
		"192.168.1.1": true, "*.example.com": true, "bad_label.example.com": true, "-x.example.com": true,
	} {
		if err := ValidateCustomBase(base); (err != nil) != wantErr {
			t.Errorf("ValidateCustomBase(%q) err=%v, wantErr=%v", base, err, wantErr)
		}
	}
}

func TestFallbackResolverOverride(t *testing.T) {
	r := NewFallbackResolver("203.0.113.5", ProviderChoice{Provider: "nip.io"}, map[string]ProviderChoice{"api": {Provider: "traefik.me"}})
	if got, _ := r.Domain("web"); got != "web.203-0-113-5.nip.io" {
		t.Errorf("default app = %q", got)
	}
	if got, _ := r.Domain("api"); got != "api.203-0-113-5.traefik.me" {
		t.Errorf("override app = %q", got)
	}
}
