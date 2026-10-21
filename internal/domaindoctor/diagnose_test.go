package domaindoctor

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

func TestPlanRecords(t *testing.T) {
	cases := []struct {
		name    string
		domain  string
		ips     []string
		host    string
		want    []Record
		wantNil bool
	}{
		{name: "apex uses A", domain: "example.com", ips: []string{serverIP}, host: "lr.example.net", want: []Record{{Name: "example.com", Type: "A", Value: serverIP}}},
		{name: "two label public suffix apex uses A", domain: "example.co.uk", ips: []string{serverIP}, host: "lr.example.net", want: []Record{{Name: "example.co.uk", Type: "A", Value: serverIP}}},
		{name: "subdomain with host uses CNAME", domain: "app.example.com", ips: []string{serverIP}, host: "lr.example.net", want: []Record{{Name: "app.example.com", Type: "CNAME", Value: "lr.example.net"}}},
		{name: "wildcard is a subdomain", domain: "*.example.com", ips: []string{serverIP}, host: "lr.example.net", want: []Record{{Name: "*.example.com", Type: "CNAME", Value: "lr.example.net"}}},
		{name: "subdomain with IP host uses A", domain: "app.example.com", ips: []string{serverIP}, host: serverIP, want: []Record{{Name: "app.example.com", Type: "A", Value: serverIP}}},
		{name: "subdomain without host uses A", domain: "app.example.com", ips: []string{serverIP}, want: []Record{{Name: "app.example.com", Type: "A", Value: serverIP}}},
		{name: "ipv6 only uses AAAA", domain: "app.example.com", ips: []string{"2001:db8::7"}, want: []Record{{Name: "app.example.com", Type: "AAAA", Value: "2001:db8::7"}}},
		{name: "prefers IPv4", domain: "app.example.com", ips: []string{"2001:db8::7", serverIP}, want: []Record{{Name: "app.example.com", Type: "A", Value: serverIP}}},
		{name: "nothing known", domain: "app.example.com", wantNil: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PlanRecords(tc.domain, tc.ips, tc.host)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsCloudflareIP(t *testing.T) {
	cases := map[string]bool{
		"104.21.5.9":      true,
		"172.67.1.2":      true,
		"2606:4700::6810": true,
		"203.0.113.7":     false,
		"8.8.8.8":         false,
		"not an ip":       false,
	}
	for ip, want := range cases {
		if got := IsCloudflareIP(ip); got != want {
			t.Errorf("IsCloudflareIP(%q) = %v, want %v", ip, got, want)
		}
	}
}

func TestExplainACME(t *testing.T) {
	cases := []struct {
		name, raw, want string
		action          string
	}{
		{"rate limit", "urn:ietf:params:acme:error:rateLimited: too many certificates already issued", "rate limiting", ""},
		{"caa", "Error finding CAA records: CAA record for example.com prevents issuance", "CAA record", ""},
		{"dns-01 permission", "solving challenge: dns-01: adding temporary record for zone: HTTP 403 forbidden", "token", ACMEActionFixDNSAccess},
		{"dns missing", "DNS problem: NXDOMAIN looking up A for example.com", "could not find DNS records", ""},
		{"other proxy", "Invalid response from http://example.com/.well-known/acme-challenge/x: 404", "Something other than this platform", ""},
		{"challenge failed", "Timeout during connect (likely firewall problem)", "challenge failed", ""},
		{"unknown", "boom", "refused to issue", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExplainACME(tc.raw); !strings.Contains(got, tc.want) {
				t.Fatalf("ExplainACME = %q, want it to contain %q", got, tc.want)
			}
			got := effectiveACMEAction(&ACMEFailureFacts{Error: tc.raw, Action: "check_logs"})
			wantAction := tc.action
			if wantAction == "" {
				wantAction = "check_logs"
			}
			if got != wantAction {
				t.Fatalf("action = %q, want %q", got, wantAction)
			}
		})
	}
}

func runWith(t *testing.T, facts Facts, dns fakeDNS) Report {
	t.Helper()
	tc, hc := new(int), new(int)
	probes := Probes{DNS: dns, TLS: fakeTLS{cert: goodCert(), calls: tc}, HTTP: fakeHTTP{responses: redirectToHTTPS(), calls: hc, dialed: new([]string)}}
	return Run(context.Background(), facts, probes, Options{Now: func() time.Time { return testNow }})
}

func check(t *testing.T, rep Report, id string) Check {
	t.Helper()
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %s in %v", id, rep.Checks)
	return Check{}
}

func TestNotResolvingFixKinds(t *testing.T) {
	missing := fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}, {Resolver: "1.1.1.1", Error: "not found"}}}
	cases := []struct {
		name     string
		mutate   func(*Facts)
		wantKind fixit.Kind
		wantAct  string
		wantAPI  string
	}{
		{"provider connected creates the record", func(f *Facts) { f.DNSProvider, f.DNSZone, f.CLIName = "cloudflare", "example.com.", "cli" }, fixit.KindAuto, ActionRequest, "POST /api/v1/apps/shop/domains/shop.example.com/dns-records"},
		{"no provider shows exact values", func(*Facts) {}, fixit.KindGuided, ActionCopy, ""},
		{"standalone without provider", func(f *Facts) { f.App = "" }, fixit.KindGuided, ActionCopy, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFacts()
			tc.mutate(&f)
			fix := check(t, runWith(t, f, missing), IDDNSResolves).Fix
			if fix == nil || fix.Kind != tc.wantKind || fix.Action == nil || fix.Action.Kind != tc.wantAct {
				t.Fatalf("fix = %+v", fix)
			}
			if tc.wantAPI != "" {
				if fix.Action.API != tc.wantAPI {
					t.Fatalf("api = %q", fix.Action.API)
				}
				if fix.Action.Body["name"] != "shop" || fix.Action.Body["type"] != "A" || fix.Action.Body["value"] != serverIP {
					t.Fatalf("body = %v", fix.Action.Body)
				}
				if fix.CLI != "cli domains diagnose shop.example.com --fix" {
					t.Fatalf("cli = %q", fix.CLI)
				}
			}
			if len(fix.Records) != 1 || fix.Records[0].Type != "A" {
				t.Fatalf("records = %v", fix.Records)
			}
		})
	}
}

func TestFixKindClassification(t *testing.T) {
	cases := []struct {
		name string
		fix  *Fix
		want fixit.Kind
	}{
		{"write request is auto", &Fix{Action: &Action{Kind: ActionRequest, API: "POST /x"}}, fixit.KindAuto},
		{"read request is guided", &Fix{Action: &Action{Kind: ActionRequest, API: "GET /x"}}, fixit.KindGuided},
		{"link is guided", &Fix{Action: &Action{Kind: ActionLink, API: "/apps"}}, fixit.KindGuided},
		{"records are guided", &Fix{Records: []Record{{Type: "A"}}}, fixit.KindGuided},
		{"text only is manual", &Fix{Summary: "wait"}, fixit.KindManual},
	}
	rn := &run{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Check{Fix: tc.fix}
			rn.finishFix(&c)
			if c.Fix.Kind != tc.want {
				t.Fatalf("kind = %s, want %s", c.Fix.Kind, tc.want)
			}
		})
	}
}

func TestCloudflareProxyDetected(t *testing.T) {
	cf := fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", IPv4: []string{"104.21.5.9"}}, {Resolver: "1.1.1.1", IPv4: []string{"172.67.1.2"}}}}
	rep := runWith(t, baseFacts(), cf)
	if got := check(t, rep, IDDNSProxied); got.State != StateWarn || !strings.Contains(got.Title, "Cloudflare") || got.Fix == nil || got.Fix.Kind != fixit.KindManual {
		t.Fatalf("proxied = %+v", got)
	}
	if got := check(t, rep, IDDNSPointsHere); got.State != StateSkipped {
		t.Fatalf("points_here = %+v", got)
	}
	if rep.Probed {
		t.Fatal("must not connect to Cloudflare addresses")
	}
	if got := check(t, runWith(t, baseFacts(), fakeDNS{answers: here()}), IDDNSProxied); got.State != StatePass {
		t.Fatalf("direct domain flagged: %+v", got)
	}
}

func TestCloudflareCNAMETarget(t *testing.T) {
	d := fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", IPv4: []string{"198.51.100.9"}}}, cname: "shop.example.com.cdn.cloudflare.net"}
	if got := check(t, runWith(t, baseFacts(), d), IDDNSProxied); got.State != StateWarn {
		t.Fatalf("proxied = %+v", got)
	}
}

func TestWildcard(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		mutate func(*Facts)
		dns    fakeDNS
		want   State
		detail string
	}{
		{"wildcard record answers", "shop.example.com", nil, fakeDNS{answers: here()}, StatePass, "wildcard record under example.com answers"},
		{"wildcard domain needs provider", "*.example.com", nil, fakeDNS{answers: here()}, StateWarn, "DNS-01"},
		{"wildcard domain with provider", "*.example.com", func(f *Facts) { f.DNSProvider = "route53" }, fakeDNS{answers: here()}, StatePass, "Route 53"},
		{"wildcard domain behind own proxy", "*.example.com", func(f *Facts) { f.TLSTerminatedUpstream = true }, fakeDNS{answers: here()}, StateSkipped, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFacts()
			f.Domain = tc.domain
			if tc.mutate != nil {
				tc.mutate(&f)
			}
			got := check(t, runWith(t, f, tc.dns), IDDNSWildcard)
			if got.State != tc.want || !strings.Contains(got.Detail, tc.detail) {
				t.Fatalf("wildcard = %+v", got)
			}
		})
	}
}

func TestPropagation(t *testing.T) {
	cases := []struct {
		name  string
		dns   fakeDNS
		state string
		next  int
	}{
		{"settled", fakeDNS{answers: here()}, PropagationSettled, 0},
		{"waiting for a new record", fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}}}, PropagationWaiting, 30},
		{"disagreeing resolvers wait out the ttl", fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}, {Resolver: "1.1.1.1", IPv4: []string{serverIP}, TTL: 120}}}, PropagationPropagating, 120},
		{"ttl floor", fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}, {Resolver: "1.1.1.1", IPv4: []string{serverIP}, TTL: 2}}}, PropagationPropagating, 15},
		{"ttl ceiling", fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}, {Resolver: "1.1.1.1", IPv4: []string{serverIP}, TTL: 86400}}}, PropagationPropagating, 300},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := runWith(t, baseFacts(), tc.dns).Propagation
			if p == nil || p.State != tc.state || p.NextCheckSeconds != tc.next {
				t.Fatalf("propagation = %+v", p)
			}
			if tc.next > 0 && !p.NextCheckAt.Equal(testNow.Add(time.Duration(tc.next)*time.Second)) {
				t.Fatalf("next_check_at = %s", p.NextCheckAt)
			}
		})
	}
}

func TestACMEFailureDetailIsPlainLanguage(t *testing.T) {
	f := baseFacts()
	f.ACMEFailure = &ACMEFailureFacts{Error: "dns-01: HTTP 403 forbidden", Action: "check_logs"}
	f.DNSProvider = "cloudflare"
	got := check(t, runWith(t, f, fakeDNS{answers: here()}), IDStoredCert)
	if got.State != StateFail || !strings.Contains(got.Detail, "refused the platform's token") || !strings.Contains(got.Detail, "CA said: dns-01: HTTP 403 forbidden") {
		t.Fatalf("stored cert = %+v", got)
	}
	if got.Fix == nil || got.Fix.Action == nil || got.Fix.Action.Kind != ActionLink || got.Fix.Kind != fixit.KindGuided {
		t.Fatalf("fix = %+v", got.Fix)
	}
}

func TestRunStreamEmitsEveryCheckOnce(t *testing.T) {
	var mu sync.Mutex
	var got []string
	tc, hc := new(int), new(int)
	probes := Probes{DNS: fakeDNS{answers: here()}, TLS: fakeTLS{cert: goodCert(), calls: tc}, HTTP: fakeHTTP{responses: redirectToHTTPS(), calls: hc, dialed: new([]string)}}
	rep := RunStream(context.Background(), baseFacts(), probes, Options{Now: func() time.Time { return testNow }}, func(c Check) {
		mu.Lock()
		got = append(got, c.ID)
		mu.Unlock()
	})
	if got[0] != IDDNSResolves {
		t.Fatalf("first emitted = %s, want %s", got[0], IDDNSResolves)
	}
	var ids []string
	for _, c := range rep.Checks {
		ids = append(ids, c.ID)
	}
	slices.Sort(got)
	slices.Sort(ids)
	if !slices.Equal(got, ids) {
		t.Fatalf("emitted %v, report %v", got, ids)
	}
	if rep.Checks[0].ID != IDDNSResolves || rep.Checks[len(rep.Checks)-1].ID != IDStoredCert {
		t.Fatalf("report not in canonical order: %v", ids)
	}
}
