package trafficpolicy

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestGeoDecideMatrix(t *testing.T) {
	allow := Geo{Mode: GeoAllow, Countries: []string{"US", "CA"}, Action: GeoActionBlock}
	deny := Geo{Mode: GeoDeny, Countries: []string{"RU"}, Action: GeoActionBlock}
	strict := allow
	strict.Unknown = GeoUnknownBlock
	public := netip.MustParseAddr("203.0.113.9")
	exempt := []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	tests := []struct {
		name    string
		g       Geo
		addr    string
		country string
		blocked bool
	}{
		{"allow listed", allow, public.String(), "us", false},
		{"allow unlisted", allow, public.String(), "DE", true},
		{"deny listed", deny, public.String(), "RU", true},
		{"deny unlisted", deny, public.String(), "DE", false},
		{"unknown allowed by default", allow, public.String(), "", false},
		{"unknown blocked when strict", strict, public.String(), "", true},
		{"garbage country is unknown", strict, public.String(), "XX", true},
		{"private never blocked", strict, "10.1.2.3", "DE", false},
		{"loopback never blocked", deny, "127.0.0.1", "RU", false},
		{"ipv6 loopback never blocked", deny, "::1", "RU", false},
		{"cgnat never blocked", deny, "100.64.1.1", "RU", false},
		{"mapped private never blocked", deny, "::ffff:192.168.1.1", "RU", false},
		{"exempt range", deny, "198.51.100.7", "RU", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.g.Decide(netip.MustParseAddr(tt.addr), tt.country, exempt)
			if d.Blocked != tt.blocked {
				t.Fatalf("Decide() = %+v, want blocked %v", d, tt.blocked)
			}
		})
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func TestEgressGuard(t *testing.T) {
	res := fakeResolver{
		"public.example.org": {netip.MustParseAddr("93.184.216.34")},
		"mixed.example.org":  {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.5")},
		"meta.example.org":   {netip.MustParseAddr("169.254.169.254")},
	}
	tests := []struct {
		name  string
		guard EgressGuard
		url   string
		ok    bool
		dial  string
	}{
		{"public name", EgressGuard{}, "https://public.example.org/base", true, "93.184.216.34:443"},
		{"http port default", EgressGuard{}, "http://public.example.org", true, "93.184.216.34:80"},
		{"explicit port", EgressGuard{}, "https://93.184.216.34:8443", true, "93.184.216.34:8443"},
		{"loopback refused", EgressGuard{}, "http://127.0.0.1:8080", false, ""},
		{"loopback allowed", EgressGuard{AllowLoopback: true}, "http://127.0.0.1:8080", true, "127.0.0.1:8080"},
		{"private refused", EgressGuard{}, "http://10.0.0.5", false, ""},
		{"private allowed", EgressGuard{AllowPrivate: true}, "http://10.0.0.5", true, "10.0.0.5:80"},
		{"metadata always refused", EgressGuard{AllowPrivate: true, AllowLoopback: true}, "http://169.254.169.254/latest", false, ""},
		{"metadata by name refused", EgressGuard{AllowPrivate: true}, "http://meta.example.org", false, ""},
		{"any private record refuses", EgressGuard{}, "https://mixed.example.org", false, ""},
		{"ipv6 link local refused", EgressGuard{AllowPrivate: true}, "http://[fe80::1]", false, ""},
		{"unspecified refused", EgressGuard{AllowPrivate: true, AllowLoopback: true}, "http://0.0.0.0", false, ""},
		{"unresolvable", EgressGuard{}, "https://nope.example.org", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.guard.CheckTarget(context.Background(), res, tt.url)
			if (err == nil) != tt.ok {
				t.Fatalf("CheckTarget() err = %v, want ok %v", err, tt.ok)
			}
			if tt.ok && got.Dial() != tt.dial {
				t.Fatalf("Dial() = %s, want %s", got.Dial(), tt.dial)
			}
		})
	}
	g := EgressGuardFromEnv(func(k string) string {
		return map[string]string{EnvForwardAllowPrivate: "true"}[k]
	})
	if !g.AllowPrivate || g.AllowLoopback {
		t.Fatalf("EgressGuardFromEnv() = %+v", g)
	}
}

func TestExplain(t *testing.T) {
	p := Policy{
		Redirects: &Redirects{TrailingSlash: SlashRemove},
		Geo:       &Geo{Mode: GeoDeny, Countries: []string{"RU"}, Action: GeoActionBlock},
		Headers: &Headers{
			Rules: []HeaderRule{{Side: SideRequest, Op: OpSet, Name: "X-Env", Value: "prod"}, {Side: SideResponse, Op: OpRemove, Name: "X-Debug"}},
			CORS:  &CORSPreset{Origins: []string{"https://a.example.com"}, Preflight: true},
		},
		Cache: &Cache{Enabled: true, Rules: []CacheRule{{Match: Match{Kind: MatchPrefix, Path: "/assets"}, TTLSeconds: 60}}},
		Forwarders: &Forwarders{Rules: []Forwarder{
			{Match: Match{Kind: MatchPrefix, Path: "/api"}, Action: ActionApp, App: "api", StripPrefix: true, RewritePrefix: "/v2"},
			{Match: Match{Kind: MatchExact, Path: "/old"}, Action: ActionRedirect, URL: "https://x.org"},
		}},
	}
	tests := []struct {
		method, path string
		last         string
		contains     []string
	}{
		{"GET", "/api/users", "proxied to app api as /v2/users", []string{"RU are stopped", "X-Env", "not cached"}},
		{"GET", "/apix", "this domain's app", nil},
		{"GET", "/assets/a.css", "this domain's app", []string{"cache rule 1"}},
		{"GET", "/old", "302 redirect to https://x.org", nil},
		{"GET", "/docs/", "308 redirect to /docs", nil},
		{"OPTIONS", "/api/x", "preflight", nil},
		{"POST", "/assets/a.css", "this domain's app", []string{"only GET and HEAD"}},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			steps := Explain(p, tt.method, tt.path)
			var all []string
			final := ""
			for _, s := range steps {
				all = append(all, s.Outcome)
				if s.Final {
					final = s.Outcome
				}
			}
			joined := strings.Join(all, " | ")
			if !strings.Contains(final, tt.last) {
				t.Fatalf("final = %q, want %q (all: %s)", final, tt.last, joined)
			}
			for _, c := range tt.contains {
				if !strings.Contains(joined, c) {
					t.Errorf("steps %q missing %q", joined, c)
				}
			}
		})
	}
}

func TestSimulateHops(t *testing.T) {
	base := HopEnv{
		Hosts:         map[string]bool{"example.com": true, "www.example.com": true, "old.example.com": true},
		Redirects:     map[string]Redirects{},
		HostRedirects: map[string]HostRedirect{"www.example.com": {TargetURL: "https://example.com", StatusCode: 301}},
		AutoHTTPS:     true,
		HTTPSPort:     443,
	}
	terminated := base
	terminated.AutoHTTPS, terminated.Terminated, terminated.HTTPSPort = false, true, 8443
	terminated.Redirects = map[string]Redirects{"www.example.com": {ForceHTTPS: true}, "example.com": {ForceHTTPS: true, ForceHTTPSStatus: 301}}
	loop := base
	loop.HostRedirects = map[string]HostRedirect{"www.example.com": {TargetURL: "https://example.com"}, "example.com": {TargetURL: "https://www.example.com"}}
	tests := []struct {
		name    string
		env     HopEnv
		url     string
		want    []int
		lastLoc string
		wantErr bool
	}{
		{"www http to apex", base, "http://www.example.com/a?x=1", []int{308, 301, 200}, "https://example.com/a?x=1", false},
		{"apex http", base, "http://example.com/a", []int{308, 200}, "https://example.com/a", false},
		{"www https", base, "https://www.example.com/a", []int{301, 200}, "https://example.com/a", false},
		{"terminated keeps public port", terminated, "http://example.com/a", []int{301, 200}, "https://example.com:8443/a", false},
		{"loop detected", loop, "https://www.example.com/", nil, "", true},
		{"foreign host", base, "https://other.org/", []int{0}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hops, err := SimulateHops(tt.env, tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SimulateHops() err = %v, want err %v (hops %+v)", err, tt.wantErr, hops)
			}
			if tt.wantErr {
				return
			}
			var got []int
			lastLoc := ""
			for _, h := range hops {
				got = append(got, h.Status)
				if h.Location != "" {
					lastLoc = h.Location
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("statuses = %v, want %v (%+v)", got, tt.want, hops)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("statuses = %v, want %v", got, tt.want)
				}
			}
			if lastLoc != tt.lastLoc {
				t.Fatalf("last location = %q, want %q", lastLoc, tt.lastLoc)
			}
		})
	}
}

func TestCanonical(t *testing.T) {
	tests := []struct {
		domain, preset, from, to string
		wantErr                  bool
	}{
		{"example.com", PresetWWWToApex, "www.example.com", "example.com", false},
		{"www.example.com", PresetWWWToApex, "www.example.com", "example.com", false},
		{"example.com", PresetApexToWWW, "example.com", "www.example.com", false},
		{"example.com", PresetServeBoth, "", "", false},
		{"*.example.com", PresetWWWToApex, "", "", true},
		{"www.localhost", PresetWWWToApex, "", "", true},
		{"example.com", "sideways", "", "", true},
	}
	for _, tt := range tests {
		from, to, err := CanonicalPlan(tt.domain, tt.preset)
		if (err != nil) != tt.wantErr || from != tt.from || to != tt.to {
			t.Errorf("CanonicalPlan(%s, %s) = %s, %s, %v", tt.domain, tt.preset, from, to, err)
		}
	}
	rows := map[string]HostRedirect{"b.example.com": {TargetURL: "https://c.example.com"}, "c.example.com": {TargetURL: "https://a.example.com/x"}}
	if err := CheckRedirectLoop(rows, "a.example.com", "b.example.com"); err == nil {
		t.Error("CheckRedirectLoop() missed a -> b -> c -> a")
	}
	if err := CheckRedirectLoop(rows, "d.example.com", "b.example.com"); err != nil {
		t.Errorf("CheckRedirectLoop() = %v, want nil", err)
	}
	if err := CheckRedirectLoop(nil, "a.example.com", "a.example.com"); err == nil {
		t.Error("CheckRedirectLoop() allowed a self redirect")
	}
}

func TestPrefixMatchesAndForwardedPath(t *testing.T) {
	if !PrefixMatches("/api", "/api") || !PrefixMatches("/api/", "/api/x") || PrefixMatches("/api", "/apix") || !PrefixMatches("/", "/x") {
		t.Fatal("PrefixMatches is not segment aware")
	}
	f := Forwarder{Match: Match{Kind: MatchPrefix, Path: "/api"}, Action: ActionURL, URL: "https://x.org/base", StripPrefix: true}
	if got := ForwardedPath(f, "/api"); got != "/base/" {
		t.Fatalf("ForwardedPath() = %q", got)
	}
	if got := ForwardedPath(f, "/api/v1"); got != "/base/v1" {
		t.Fatalf("ForwardedPath() = %q", got)
	}
}
