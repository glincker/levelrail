package ingress

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var updateStandaloneGolden = flag.Bool("update-standalone-golden", false, "rewrite testdata/standalone_routes.golden.json")

// standaloneGoldenRoutes is one route per standalone target kind as the
// reconciler hands them over: the dial is already resolved.
func standaloneGoldenRoutes() []ProxyRoute {
	return []ProxyRoute{
		{Hosts: []string{"port.example.com"}, BackendDial: "127.0.0.1:3000", Upstream: &UpstreamOptions{}},
		{Hosts: []string{"address.example.com"}, BackendDial: "10.0.0.5:8080", Upstream: &UpstreamOptions{
			DialTimeout: 5 * time.Second, ResponseHeaderTimeout: 90 * time.Second, StripPrefix: "/app",
		}},
		{Hosts: []string{"url.example.com"}, BackendDial: "203.0.113.9:8443", Upstream: &UpstreamOptions{
			TLS: true, ServerName: "upstream.example.org", HostHeader: "upstream.example.org:8443", BasePath: "/base",
		}},
		{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:49153", Upstream: &UpstreamOptions{DisableWebSocket: true}},
		{Hosts: []string{"container.example.com"}, BackendDial: "172.18.0.4:8081", Upstream: &UpstreamOptions{
			TLS: true, InsecureSkipVerify: true, HostFromUpstream: true,
		}},
	}
}

func TestStandaloneRoutesGolden(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "srv", ListenAddr: ":443", Routes: standaloneGoldenRoutes()})
	if err != nil {
		t.Fatalf("BuildRoutesConfig: %v", err)
	}
	got, err := json.MarshalIndent(cfg.Apps.HTTP.Servers["srv"].Routes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "standalone_routes.golden.json")
	if *updateStandaloneGolden {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("read golden (run with -update-standalone-golden): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("standalone routes differ from %s; rerun with -update-standalone-golden and review\n%s", path, got)
	}
}

func TestUpstreamOptionsMergeWithHardening(t *testing.T) {
	h := DefaultHardening()
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "srv", ListenAddr: ":443", Hardening: &h,
		Routes: []ProxyRoute{{Hosts: []string{"a.example.com"}, BackendDial: "127.0.0.1:3000", Upstream: &UpstreamOptions{
			TLS: true, HostHeader: "{evil}", DisableWebSocket: true, ResponseHeaderTimeout: time.Minute,
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle := cfg.Apps.HTTP.Servers["srv"].Routes[0].Handle
	rp, ok := handle[len(handle)-1].(ReverseProxyHandler)
	if !ok {
		t.Fatalf("last handler is %T, want reverse proxy", handle[len(handle)-1])
	}
	if rp.Transport == nil || rp.Transport.TLS == nil || rp.Transport.ResponseHeaderTimeout != "1m0s" || rp.Transport.DialTimeout == "" {
		t.Fatalf("transport = %+v, want TLS, read timeout and the hardening dial timeout kept", rp.Transport)
	}
	req := rp.Headers.Request
	if req.Set["Host"][0] != `\{evil\}` || req.Set["X-Real-IP"] == nil || len(req.Delete) != 1 {
		t.Fatalf("headers = %+v, want escaped host, X-Real-IP kept and Upgrade deleted", req)
	}
}

func TestUpstreamOptionsIgnoredForLB(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "srv", ListenAddr: ":443", Routes: []ProxyRoute{{
		Hosts: []string{"a.example.com"}, BackendDial: "127.0.0.1:1",
		LB:       &LBRoute{Upstreams: []string{"127.0.0.1:1", "127.0.0.1:2"}},
		Upstream: &UpstreamOptions{StripPrefix: "/x"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(cfg.Apps.HTTP.Servers["srv"].Routes[0].Handle); n != 1 {
		t.Fatalf("LB route has %d handlers, want only the reverse proxy", n)
	}
}
