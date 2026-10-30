package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestReachableIPv4s(t *testing.T) {
	ips := reachableIPv4s(map[string][]net.IP{
		"lo":      {net.ParseIP("127.0.0.1")},
		"docker0": {net.ParseIP("172.17.0.1")},
		"br-abc":  {net.ParseIP("172.18.0.1")},
		"veth1":   {net.ParseIP("10.9.9.9")},
		"eth0":    {net.ParseIP("192.168.1.20"), net.ParseIP("fe80::1"), net.ParseIP("169.254.3.3")},
		"eth1":    {net.ParseIP("203.0.113.7")},
	})
	if len(ips) != 2 || ips[0].String() != "192.168.1.20" || ips[1].String() != "203.0.113.7" {
		t.Fatalf("got %v", ips)
	}
}

func TestHostPortAddresses(t *testing.T) {
	ips := []net.IP{net.ParseIP("192.168.1.20").To4(), net.ParseIP("203.0.113.7").To4()}
	cases := []struct {
		name     string
		port     int
		running  bool
		bind     string
		direct   bool
		public   string
		wantKind []string
		wantHint string
	}{
		{"localhost bound", 3333, true, "127.0.0.1", false, "", []string{"loopback", "lan"}, "bound_localhost"},
		{"all interfaces", 3333, true, "0.0.0.0", true, "198.51.100.1", []string{"loopback", "lan", "public", "public"}, "reachable"},
		{"stopped", 0, false, "127.0.0.1", false, "", []string{"loopback", "lan"}, "bound_localhost"},
		{"specific ip", 3333, true, "192.168.1.20", false, "", []string{"loopback", "lan"}, "reachable"},
		{"stopped public bind", 3333, false, "0.0.0.0", false, "", []string{"loopback", "lan", "public"}, "not_running"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hostPortAddresses(c.port, c.running, c.bind, ips, c.public, c.direct)
			if len(got) != len(c.wantKind) {
				t.Fatalf("got %+v", got)
			}
			for i, k := range c.wantKind {
				if got[i].Kind != k {
					t.Fatalf("row %d kind %s want %s", i, got[i].Kind, k)
				}
			}
			if got[1].ReachableHint != c.wantHint {
				t.Fatalf("hint %s want %s", got[1].ReachableHint, c.wantHint)
			}
			if c.bind == "127.0.0.1" && got[1].URL != "" {
				t.Fatal("localhost-bound LAN row must not carry a url")
			}
			if c.port == 0 && got[0].URL != "" {
				t.Fatal("loopback url without a host port")
			}
		})
	}
}

func TestGetAppURLs(t *testing.T) {
	prev := hostInterfaceAddrs
	hostInterfaceAddrs = func() map[string][]net.IP {
		return map[string][]net.IP{"eth0": {net.ParseIP("10.0.0.5")}}
	}
	t.Cleanup(func() { hostInterfaceAddrs = prev })
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	hp := 24100
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "i:1", Port: 3000, HostPort: &hp, BindAddress: "public"}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/urls", ""))
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var res appAccessResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.ContainerPort != 3000 || res.HostPort != 24100 || res.Running || !res.LocalNode {
		t.Fatalf("got %+v", res)
	}
	var internal, lan bool
	for _, a := range res.Addresses {
		internal = internal || (a.Kind == "internal" && a.Port == 3000)
		lan = lan || (a.Kind == "lan" && a.URL == "http://10.0.0.5:24100" && a.ReachableHint == "not_running")
	}
	if !internal || !lan {
		t.Fatalf("missing rows: %+v", res.Addresses)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nope/urls", ""))
	if rec.Code != 404 {
		t.Fatalf("unknown app: %d", rec.Code)
	}
}

func TestGetAppURLsHTTPFallback(t *testing.T) {
	cases := []struct {
		name    string
		domains []string
		ing     store.IngressSettings
		httpP   int
		want    string
	}{
		{"redirect ingress has no plain route", nil, store.IngressSettings{}, 0, ""},
		{"acme issued has nothing new", nil, store.IngressSettings{ACMEEnabled: true, ACMEEmail: "a@b.co"}, 0, ""},
		{"custom domain unchanged", []string{"x.example.com"}, store.IngressSettings{TLSTerminatedUpstream: true}, 0, ""},
		{"fallback disabled", nil, store.IngressSettings{TLSTerminatedUpstream: true, FallbackDomainsDisabled: true}, 0, ""},
		{"proxy fronted default port", nil, store.IngressSettings{TLSTerminatedUpstream: true}, 0, "http://web.203-0-113-5.sslip.io"},
		{"proxy fronted custom port", nil, store.IngressSettings{TLSTerminatedUpstream: true}, 8080, "http://web.203-0-113-5.sslip.io:8080"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			rt.publicHost = "203.0.113.5"
			rt.doctorHTTPPort = c.httpP
			cookie := loginTestSession(t, rt, db)
			ctx := context.Background()
			if err := db.UpdateIngressSettings(ctx, c.ing); err != nil {
				t.Fatal(err)
			}
			if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "i:1", Port: 3000, BindAddress: "private", Domains: c.domains}); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/urls", ""))
			if rec.Code != 200 {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
			var res appAccessResource
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			if res.HTTPFallbackURL != c.want {
				t.Fatalf("http_fallback_url = %q, want %q", res.HTTPFallbackURL, c.want)
			}
		})
	}
}
