package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedirectLocation(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{"bare origin keeps the request uri", "https://example.com", "https://example.com{http.request.uri}"},
		{"trailing slash origin keeps the request uri", "https://example.com/", "https://example.com{http.request.uri}"},
		{"origin with port keeps the request uri", "https://example.com:8443", "https://example.com:8443{http.request.uri}"},
		{"target with a path stays fixed", "https://newapp.example.com/promo", "https://newapp.example.com/promo"},
		{"target with a query stays fixed", "https://example.com/?ref=old", "https://example.com/?ref=old"},
		{"target with a fragment stays fixed", "https://example.com#top", "https://example.com#top"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redirectLocation(tt.target); got != tt.want {
				t.Errorf("redirectLocation(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestHTTPSRedirectLocationKeepsCustomPort(t *testing.T) {
	if got := httpsRedirectLocation(443); got != "https://{http.request.host}{http.request.uri}" {
		t.Errorf("443 = %q", got)
	}
	if got := httpsRedirectLocation(0); got != "https://{http.request.host}{http.request.uri}" {
		t.Errorf("0 = %q", got)
	}
	if got := httpsRedirectLocation(18703); got != "https://{http.request.host}:18703{http.request.uri}" {
		t.Errorf("18703 = %q", got)
	}
	if listenPort(":18703") != 18703 || listenPort("0.0.0.0:443") != 443 || listenPort("fd/3") != 0 {
		t.Error("listenPort parse")
	}
}

func TestRoutesRedirectServerUsesListenPort(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "edge", TLS: true, HTTPRedirect: true, ListenAddr: ":18703", HTTPPort: 18702,
		Routes: []ProxyRoute{{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg.Apps.HTTP.Servers["edge-http"])
	if !strings.Contains(string(raw), "{http.request.host}:18703{http.request.uri}") {
		t.Errorf("redirect drops the https port: %s", raw)
	}
}
