package dnszones

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"
)

type fakeAuthResolver struct {
	ns     []string
	nsErr  error
	ips    map[string][]netip.Addr
	ipErr  error
	ttl    time.Duration
	hasTTL bool
}

func (f *fakeAuthResolver) LookupNS(context.Context, string) ([]string, error) { return f.ns, f.nsErr }
func (f *fakeAuthResolver) LookupIP(_ context.Context, n string) ([]netip.Addr, error) {
	return f.ips[n], f.ipErr
}
func (f *fakeAuthResolver) LookupCNAME(_ context.Context, n string) (string, error) {
	return n + ".", nil
}
func (f *fakeAuthResolver) LookupTTL(context.Context, string) (time.Duration, bool, error) {
	return f.ttl, f.hasTTL, nil
}

func TestCheckAuthority(t *testing.T) {
	a := netip.MustParseAddr
	local := []netip.Addr{a("203.0.113.9")}
	cases := []struct {
		name      string
		res       *fakeAuthResolver
		warn      time.Duration
		providers []string
		risks     []string
		proxied   bool
	}{
		{"cloudflare", &fakeAuthResolver{ns: []string{"amy.ns.cloudflare.com.", "bob.ns.cloudflare.com."}, ips: map[string][]netip.Addr{"example.com": {a("198.51.100.1")}}},
			time.Minute, []string{"cloudflare"}, nil, false},
		{"route53", &fakeAuthResolver{ns: []string{"ns-1.awsdns-01.org"}, ips: map[string][]netip.Addr{"example.com": {a("198.51.100.1")}}},
			time.Minute, []string{"route53"}, nil, false},
		{"mixed", &fakeAuthResolver{ns: []string{"amy.ns.cloudflare.com", "ns-1.awsdns-01.org", "ns-cloud-a1.googledomains.com", "ns1.example.net"}, ips: map[string][]netip.Addr{"example.com": {a("198.51.100.1")}}},
			time.Minute, []string{"cloudflare", "google", "other", "route53"}, nil, false},
		{"this host", &fakeAuthResolver{ns: []string{"ns1.example.com"}, ips: map[string][]netip.Addr{"ns1.example.com": {a("203.0.113.9")}, "example.com": {a("203.0.113.9")}}},
			time.Minute, []string{"this_host"}, []string{RiskNSOnThisHost}, false},
		{"proxied", &fakeAuthResolver{ns: []string{"amy.ns.cloudflare.com"}, ips: map[string][]netip.Addr{"example.com": {a("104.16.1.1"), a("2606:4700::1")}}},
			time.Minute, []string{"cloudflare"}, []string{RiskProxied}, true},
		{"ttl over", &fakeAuthResolver{ns: []string{"amy.ns.cloudflare.com"}, ips: map[string][]netip.Addr{"example.com": {a("198.51.100.1")}}, ttl: time.Hour, hasTTL: true},
			time.Minute, []string{"cloudflare"}, []string{RiskTTLHigh}, false},
		{"ttl under", &fakeAuthResolver{ns: []string{"amy.ns.cloudflare.com"}, ips: map[string][]netip.Addr{"example.com": {a("198.51.100.1")}}, ttl: time.Second, hasTTL: true},
			time.Minute, []string{"cloudflare"}, nil, false},
		{"no ns", &fakeAuthResolver{ips: map[string][]netip.Addr{"example.com": {a("198.51.100.1")}}},
			time.Minute, []string{}, []string{RiskNoNS}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckAuthority(context.Background(), "Example.com.", AuthorityOptions{Resolver: tc.res, LocalIPs: local, WarnTTL: tc.warn})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got.Providers, tc.providers) {
				t.Fatalf("providers = %v, want %v", got.Providers, tc.providers)
			}
			want := tc.risks
			if want == nil {
				want = []string{}
			}
			if !slices.Equal(got.Risks, want) {
				t.Fatalf("risks = %v, want %v", got.Risks, want)
			}
			if got.Proxied != tc.proxied {
				t.Fatalf("proxied = %v", got.Proxied)
			}
		})
	}
}

func TestCheckAuthorityLookupFailure(t *testing.T) {
	_, err := CheckAuthority(context.Background(), "example.com", AuthorityOptions{Resolver: &fakeAuthResolver{nsErr: errors.New("servfail")}})
	if !errors.Is(err, ErrAuthorityLookup) {
		t.Fatalf("err = %v", err)
	}
	_, err = CheckAuthority(context.Background(), "example.com", AuthorityOptions{Resolver: &fakeAuthResolver{ns: []string{"a.ns.cloudflare.com"}, ipErr: errors.New("timeout")}})
	if !errors.Is(err, ErrAuthorityLookup) {
		t.Fatalf("err = %v", err)
	}
}

func TestIsCloudflareProxyAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"104.16.0.1": true, "172.67.1.1": true, "2606:4700::1": true,
		"198.51.100.1": false, "2001:db8::1": false,
	} {
		if got := IsCloudflareProxyAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s = %v, want %v", addr, got, want)
		}
	}
}
