package api

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/dnszones"
)

type fakeAuthorityResolver struct {
	ns    []string
	ips   map[string][]netip.Addr
	err   error
	calls int
}

func (f *fakeAuthorityResolver) LookupNS(context.Context, string) ([]string, error) {
	f.calls++
	return f.ns, f.err
}

func (f *fakeAuthorityResolver) LookupIP(_ context.Context, n string) ([]netip.Addr, error) {
	return f.ips[n], nil
}

func (f *fakeAuthorityResolver) LookupCNAME(_ context.Context, n string) (string, error) {
	return n + ".", nil
}

func TestValidateAuthorityDomain(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "127.0.0.1", "::1", "localhost", "intranet", "db.internal", "x.localhost", "a b.com", "http://x.com"} {
		if _, err := validateAuthorityDomain(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if d, err := validateAuthorityDomain(" App.Example.com. "); err != nil || d != "app.example.com" {
		t.Errorf("got %q %v", d, err)
	}
}

func TestHandleDNSAuthority(t *testing.T) {
	t.Parallel()
	a := netip.MustParseAddr
	cases := []struct {
		name     string
		query    string
		res      *fakeAuthorityResolver
		wantCode int
		wantBody []string
		noLookup bool
	}{
		{name: "proxied cloudflare", query: "domain=app.example.com", wantCode: http.StatusOK,
			res:      &fakeAuthorityResolver{ns: []string{"ana.ns.cloudflare.com."}, ips: map[string][]netip.Addr{"app.example.com": {a("104.16.0.1")}}},
			wantBody: []string{`"proxied":true`, `"providers":["cloudflare"]`, dnszones.RiskProxied}},
		{name: "ns on this host", query: "domain=example.com", wantCode: http.StatusOK,
			res: &fakeAuthorityResolver{ns: []string{"ns1.example.com"}, ips: map[string][]netip.Addr{
				"ns1.example.com": {a("203.0.113.9")}, "example.com": {a("203.0.113.9")}}},
			wantBody: []string{`"points_here":true`, dnszones.RiskNSOnThisHost}},
		{name: "lookup failure", query: "domain=example.com", wantCode: http.StatusBadGateway,
			res: &fakeAuthorityResolver{err: errors.New("timeout")}},
		{name: "private answers", query: "domain=corp.example.com", wantCode: http.StatusBadRequest,
			res:      &fakeAuthorityResolver{ns: []string{"ns.example.net"}, ips: map[string][]netip.Addr{"corp.example.com": {a("10.0.0.5")}}},
			wantBody: []string{"internal names"}},
		{name: "ip literal", query: "domain=10.0.0.1", wantCode: http.StatusBadRequest, res: &fakeAuthorityResolver{}, noLookup: true},
		{name: "single label", query: "domain=localhost", wantCode: http.StatusBadRequest, res: &fakeAuthorityResolver{}, noLookup: true},
		{name: "missing", query: "", wantCode: http.StatusBadRequest, res: &fakeAuthorityResolver{}, noLookup: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt, _, cookie := newDNSZoneTestRouter(t, nil)
			rt.dnsAuthority = dnsAuthorityDeps{
				resolver: tc.res,
				localIPs: func(context.Context) []netip.Addr { return []netip.Addr{a("203.0.113.9")} },
			}
			rec := serveDNS(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/dns/authority?"+tc.query, ""))
			if rec.Code != tc.wantCode {
				t.Fatalf("status %d body %s", rec.Code, rec.Body)
			}
			for _, w := range tc.wantBody {
				if !strings.Contains(rec.Body.String(), w) {
					t.Errorf("body missing %q: %s", w, rec.Body)
				}
			}
			if tc.noLookup && tc.res.calls != 0 {
				t.Errorf("rejected name still triggered %d lookups", tc.res.calls)
			}
		})
	}
}
