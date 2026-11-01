package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func dns01SingleOpts(all bool, withProvider bool, routes ...ProxyRoute) RoutesOptions {
	o := RoutesOptions{ServerName: "ingress", ListenAddr: ":443", TLS: true, ACMEEnabled: true, DNS01AllHosts: all, Routes: routes}
	if withProvider {
		o.DNSProvider = CloudflareDNSProvider{Name: "cloudflare", APIToken: "test-token"}
	}
	return o
}

func dns01SingleJSON(t *testing.T, o RoutesOptions) (string, error) {
	t.Helper()
	cfg, err := BuildRoutesConfig(o)
	if err != nil {
		return "", err
	}
	b, merr := json.Marshal(cfg.Apps.TLS)
	if merr != nil {
		t.Fatal(merr)
	}
	return string(b), nil
}

func TestDNS01SingleHosts(t *testing.T) {
	single := ProxyRoute{Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9002"}
	wild := ProxyRoute{Hosts: []string{"*.example.com"}, BackendDial: "127.0.0.1:9001"}

	t.Run("off keeps HTTP-01 for single hosts", func(t *testing.T) {
		out, err := dns01SingleJSON(t, dns01SingleOpts(false, true, single))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "dns") && strings.Contains(out, "challenges") {
			t.Fatalf("unexpected DNS-01 challenge: %s", out)
		}
	})
	t.Run("on selects DNS-01 for single hosts", func(t *testing.T) {
		out, err := dns01SingleJSON(t, dns01SingleOpts(true, true, single))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "dns") || !strings.Contains(out, "app.example.com") {
			t.Fatalf("want DNS-01 policy for single host: %s", out)
		}
	})
	t.Run("on without credentials fails closed", func(t *testing.T) {
		_, err := dns01SingleJSON(t, dns01SingleOpts(true, false, single))
		if err == nil || !strings.Contains(err.Error(), "no DNS provider") {
			t.Fatalf("want missing provider error, got %v", err)
		}
	})
	t.Run("off wildcard path unchanged", func(t *testing.T) {
		cfg, err := BuildRoutesConfig(dns01SingleOpts(false, true, wild, single))
		if err != nil {
			t.Fatal(err)
		}
		if n := len(cfg.Apps.TLS.Automation.Policies); n != 2 {
			t.Fatalf("want 2 policies, got %d", n)
		}
	})
}
