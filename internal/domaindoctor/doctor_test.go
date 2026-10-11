package domaindoctor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

const serverIP = "203.0.113.7"

type fakeDNS struct {
	answers []ResolverAnswer
	cname   string
	caa     []CAARecord
	caaErr  error
}

func (f fakeDNS) Answers(context.Context, string) []ResolverAnswer { return f.answers }
func (f fakeDNS) CNAME(context.Context, string) (string, error)    { return f.cname, nil }
func (f fakeDNS) CAA(context.Context, string) ([]CAARecord, error) { return f.caa, f.caaErr }

type fakeTLS struct {
	cert  PeerCert
	err   error
	calls *int
}

func (f fakeTLS) Handshake(_ context.Context, addr, _ string) (PeerCert, error) {
	*f.calls++
	if !strings.HasPrefix(addr, serverIP) {
		return PeerCert{}, fmt.Errorf("dialed %s, not this server", addr)
	}
	return f.cert, f.err
}

type fakeHTTP struct {
	// responses keyed by scheme://host+path
	responses map[string]HTTPResult
	err       error
	calls     *int
	dialed    *[]string
}

func (f fakeHTTP) Get(_ context.Context, addr, scheme, host, path string) (HTTPResult, error) {
	*f.calls++
	*f.dialed = append(*f.dialed, addr)
	if f.err != nil {
		return HTTPResult{}, f.err
	}
	if r, ok := f.responses[scheme+"://"+host+path]; ok {
		return r, nil
	}
	return HTTPResult{Status: 200}, nil
}

func here() []ResolverAnswer {
	return []ResolverAnswer{{Resolver: "this server", IPv4: []string{serverIP}}, {Resolver: "1.1.1.1", IPv4: []string{serverIP}}}
}

func goodCert() PeerCert {
	return PeerCert{Subject: "CN=shop.example.com", Issuer: "CN=R11,O=Let's Encrypt", DNSNames: []string{"shop.example.com"}, NotAfter: testNow.Add(80 * 24 * time.Hour), Verified: true}
}

func redirectToHTTPS() map[string]HTTPResult {
	return map[string]HTTPResult{"http://shop.example.com/": {Status: 308, Location: "https://shop.example.com/"}}
}

type scenario struct {
	name     string
	facts    func(*Facts)
	dns      fakeDNS
	tls      PeerCert
	tlsErr   error
	http     map[string]HTTPResult
	httpErr  error
	want     map[string]State
	status   string
	noProbe  bool
	contains map[string]string
}

func baseFacts() Facts {
	return Facts{
		Domain: "shop.example.com", App: "shop", ServerIPs: []string{serverIP}, ACMEEnabled: true,
		AppState: AppFacts{Known: true, Running: true, Ready: true}, PortHoldersKnown: true,
		Cert: &CertFacts{Issuer: "R11", NotAfter: testNow.Add(80 * 24 * time.Hour), Status: "healthy", Renewal: "ok", Source: "acme"},
	}
}

func TestRun(t *testing.T) {
	cases := []scenario{
		{
			name: "healthy", dns: fakeDNS{answers: here()}, tls: goodCert(), http: redirectToHTTPS(), status: StatusHealthy,
			want: map[string]State{IDDNSResolves: StatePass, IDDNSPointsHere: StatePass, IDTLS: StatePass, IDHTTPStatus: StatePass, IDRedirects: StatePass, IDHSTS: StatePass, IDStoredCert: StatePass, IDCAA: StatePass},
		},
		{
			name: "not resolving skips probes", noProbe: true, status: StatusProblems,
			dns:  fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}, {Resolver: "1.1.1.1", Error: "not found"}}},
			want: map[string]State{IDDNSResolves: StateFail, IDDNSPointsHere: StateSkipped, IDTLS: StateSkipped, IDPort80: StateSkipped},
		},
		{
			name: "resolves elsewhere refuses to probe", noProbe: true, status: StatusProblems,
			dns:      fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", IPv4: []string{"198.51.100.9"}}}},
			want:     map[string]State{IDDNSPointsHere: StateFail, IDTLS: StateSkipped, IDHTTPStatus: StateSkipped, IDRedirects: StateSkipped},
			contains: map[string]string{IDTLS: "never connects to addresses other than this server"},
		},
		{
			name: "propagating", tls: goodCert(), http: redirectToHTTPS(), status: StatusWarnings,
			dns:  fakeDNS{answers: []ResolverAnswer{{Resolver: "this server", Error: "not found"}, {Resolver: "1.1.1.1", IPv4: []string{serverIP}}}},
			want: map[string]State{IDDNSResolves: StateWarn, IDDNSPointsHere: StatePass},
		},
		{
			name: "traefik default cert behind hsts", status: StatusProblems,
			dns:      fakeDNS{answers: here()},
			tls:      PeerCert{Subject: "CN=TRAEFIK DEFAULT CERT", Issuer: "CN=TRAEFIK DEFAULT CERT", DNSNames: []string{"x.traefik.default"}, NotAfter: testNow.Add(300 * 24 * time.Hour)},
			http:     map[string]HTTPResult{"https://shop.example.com/": {Status: 404, HSTS: "max-age=31536000"}},
			want:     map[string]State{IDTLS: StateFail, IDHSTS: StateFail},
			contains: map[string]string{IDTLS: "Traefik", IDHSTS: "ERR_CERT_AUTHORITY_INVALID"},
		},
		{
			name: "preloaded tld with untrusted cert", status: StatusProblems,
			facts: func(f *Facts) { f.Domain = "shop.example.dev" },
			dns:   fakeDNS{answers: here()},
			tls:   PeerCert{Subject: "CN=shop.example.dev", Issuer: "CN=Some Private CA", DNSNames: []string{"shop.example.dev"}, NotAfter: testNow.Add(90 * 24 * time.Hour), VerifyError: "x509: unknown authority"},
			want:  map[string]State{IDTLS: StateFail, IDHSTS: StateFail}, contains: map[string]string{IDHSTS: ".dev"},
		},
		{
			name: "caddy local authority", status: StatusProblems, dns: fakeDNS{answers: here()},
			tls:  PeerCert{Subject: "", Issuer: "CN=Caddy Local Authority - ECC Intermediate", DNSNames: []string{"shop.example.com"}, NotAfter: testNow.Add(24 * time.Hour)},
			want: map[string]State{IDTLS: StateFail}, contains: map[string]string{IDTLS: "Caddy"},
		},
		{
			name: "name mismatch", status: StatusProblems, dns: fakeDNS{answers: here()},
			tls:  PeerCert{Issuer: "R11", DNSNames: []string{"other.example.com"}, NotAfter: testNow.Add(50 * 24 * time.Hour), Verified: false},
			want: map[string]State{IDTLS: StateFail}, contains: map[string]string{IDTLS: "other.example.com"},
		},
		{
			name: "aaaa mismatch", status: StatusProblems, tls: goodCert(), http: redirectToHTTPS(),
			dns:  fakeDNS{answers: []ResolverAnswer{{Resolver: "1.1.1.1", IPv4: []string{serverIP}, IPv6: []string{"2001:db8::99"}}}},
			want: map[string]State{IDDNSAAAA: StateFail},
		},
		{
			name: "caa blocks letsencrypt", status: StatusProblems, tls: goodCert(), http: redirectToHTTPS(),
			dns:  fakeDNS{answers: here(), caa: []CAARecord{{Name: "example.com", Tag: "issue", Value: "digicert.com"}}},
			want: map[string]State{IDCAA: StateFail}, contains: map[string]string{IDCAA: "digicert.com"},
		},
		{
			name: "caa lookup failure is unavailable", status: StatusHealthy, tls: goodCert(), http: redirectToHTTPS(),
			dns:  fakeDNS{answers: here(), caaErr: errors.New("timeout")},
			want: map[string]State{IDCAA: StateUnavailable},
		},
		{
			name: "apex cname", status: StatusWarnings, http: map[string]HTTPResult{"http://example.com/": {Status: 308, Location: "https://example.com/"}},
			tls:   PeerCert{Issuer: "R11", DNSNames: []string{"example.com"}, NotAfter: testNow.Add(80 * 24 * time.Hour), Verified: true},
			facts: func(f *Facts) { f.Domain = "example.com" },
			dns:   fakeDNS{answers: here(), cname: "lb.example.net"},
			want:  map[string]State{IDDNSApexCNAME: StateWarn},
		},
		{
			name: "redirect loop", status: StatusProblems, tls: goodCert(), dns: fakeDNS{answers: here()},
			http: map[string]HTTPResult{
				"http://shop.example.com/":  {Status: 301, Location: "https://shop.example.com/"},
				"https://shop.example.com/": {Status: 301, Location: "http://shop.example.com/"},
			},
			want: map[string]State{IDRedirects: StateFail}, contains: map[string]string{IDRedirects: " > "},
		},
		{
			name: "no https redirect", status: StatusWarnings, tls: goodCert(), dns: fakeDNS{answers: here()},
			want: map[string]State{IDRedirects: StateWarn},
		},
		{
			name: "port refused", status: StatusProblems, tls: goodCert(), dns: fakeDNS{answers: here()},
			httpErr: fmt.Errorf("dial: %w", syscall.ECONNREFUSED),
			want:    map[string]State{IDPort80: StateFail},
		},
		{
			name: "app stopped", status: StatusProblems, tls: goodCert(), http: redirectToHTTPS(), dns: fakeDNS{answers: here()},
			facts: func(f *Facts) { f.AppState = AppFacts{Known: true, Running: false, Detail: "suspended"} },
			want:  map[string]State{IDAppRunning: StateFail}, contains: map[string]string{IDAppRunning: "suspended"},
		},
		{
			name: "held back", status: StatusProblems, tls: goodCert(), http: redirectToHTTPS(), dns: fakeDNS{answers: here()},
			facts: func(f *Facts) { f.HeldBack = true },
			want:  map[string]State{IDHeldBack: StateFail},
		},
		{
			name: "foreign proxy on 443", status: StatusWarnings, tls: goodCert(), http: redirectToHTTPS(), dns: fakeDNS{answers: here()},
			facts: func(f *Facts) { f.PortHolders = []PortHolder{{Port: 443, Container: "traefik", Kind: "traefik"}} },
			want:  map[string]State{IDProxyPorts: StateWarn}, contains: map[string]string{IDProxyPorts: "traefik"},
		},
		{
			name: "acme failure open port 80", status: StatusProblems, tls: goodCert(), http: redirectToHTTPS(), dns: fakeDNS{answers: here()},
			facts: func(f *Facts) {
				f.ACMEFailure = &ACMEFailureFacts{Error: "connection refused", Action: ACMEActionOpenPort80, Renewal: true}
			},
			want: map[string]State{IDStoredCert: StateFail},
		},
		{
			name: "rate limited is not a failure", status: StatusHealthy, tls: goodCert(), http: redirectToHTTPS(), dns: fakeDNS{answers: here()},
			facts: func(f *Facts) { f.ACMEFailure = &ACMEFailureFacts{Error: "too many", Action: ACMEActionWait} },
			want:  map[string]State{IDStoredCert: StatePass},
		},
		{
			name: "stalled renewal", status: StatusProblems, tls: goodCert(), http: redirectToHTTPS(), dns: fakeDNS{answers: here()},
			facts: func(f *Facts) { f.Cert.Renewal = "stalled"; f.Cert.Status = "expiring_soon" },
			want:  map[string]State{IDStoredCert: StateFail},
		},
		{
			name: "upstream tls skips certificate checks", status: StatusWarnings, dns: fakeDNS{answers: []ResolverAnswer{{Resolver: "1.1.1.1", IPv4: []string{"198.51.100.9"}}}},
			facts: func(f *Facts) { f.TLSTerminatedUpstream = true }, noProbe: true,
			want: map[string]State{IDDNSPointsHere: StateWarn, IDCAA: StateSkipped, IDStoredCert: StateSkipped},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := baseFacts()
			if tc.facts != nil {
				tc.facts(&facts)
			}
			tlsCalls, httpCalls := 0, 0
			var dialed []string
			probes := Probes{
				DNS:  tc.dns,
				TLS:  fakeTLS{cert: tc.tls, err: tc.tlsErr, calls: &tlsCalls},
				HTTP: fakeHTTP{responses: tc.http, err: tc.httpErr, calls: &httpCalls, dialed: &dialed},
			}
			rep := Run(context.Background(), facts, probes, Options{Now: func() time.Time { return testNow }})
			got := map[string]Check{}
			for _, c := range rep.Checks {
				got[c.ID] = c
			}
			for id, want := range tc.want {
				if got[id].State != want {
					t.Errorf("%s = %q (%s), want %q", id, got[id].State, got[id].Detail, want)
				}
			}
			for id, sub := range tc.contains {
				if text := got[id].Title + " " + got[id].Detail; !strings.Contains(text, sub) {
					t.Errorf("%s text %q does not mention %q", id, text, sub)
				}
			}
			if tc.status != "" && rep.Status != tc.status {
				t.Errorf("status = %q, want %q", rep.Status, tc.status)
			}
			if tc.noProbe {
				if tlsCalls+httpCalls != 0 || rep.Probed {
					t.Errorf("probed a domain that does not resolve here: tls=%d http=%d", tlsCalls, httpCalls)
				}
				if rep.ProbeNote == "" {
					t.Error("probe_note is empty")
				}
			}
			for _, addr := range dialed {
				if !strings.HasPrefix(addr, serverIP) {
					t.Errorf("dialed %s, want only this server", addr)
				}
			}
			for _, c := range rep.Checks {
				if (c.State == StateFail || c.State == StateWarn) && c.Fix == nil && c.ID != IDHTTPStatus {
					t.Errorf("%s is %s without a fix", c.ID, c.State)
				}
			}
		})
	}
}

type slowDNS struct{ fakeDNS }

func (slowDNS) Answers(ctx context.Context, _ string) []ResolverAnswer {
	<-ctx.Done()
	return nil
}

func TestRunTimeout(t *testing.T) {
	calls := 0
	var dialed []string
	rep := Run(context.Background(), baseFacts(), Probes{DNS: slowDNS{}, TLS: fakeTLS{calls: &calls}, HTTP: fakeHTTP{calls: &calls, dialed: &dialed}},
		Options{Timeout: 50 * time.Millisecond, ProbeTimeout: time.Second})
	last := rep.Checks[len(rep.Checks)-1]
	if last.ID != "doctor.timeout" {
		t.Fatalf("last check = %q, want doctor.timeout", last.ID)
	}
}

func TestPrioritized(t *testing.T) {
	in := []Check{
		{ID: "hygiene", State: StateWarn, Tier: TierHygiene},
		{ID: "cert", State: StateFail, Tier: TierCertificate},
		{ID: "ok", State: StatePass, Tier: TierTraffic},
		{ID: "dns", State: StateFail, Tier: TierTraffic},
		{ID: "soon", State: StateWarn, Tier: TierSoon},
	}
	var ids []string
	for _, c := range Prioritized(in) {
		ids = append(ids, c.ID)
	}
	if got, want := strings.Join(ids, ","), "dns,cert,soon,hygiene"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}
}

func TestCAAAllows(t *testing.T) {
	cases := []struct {
		name             string
		recs             []CAARecord
		allowed, present bool
	}{
		{"none", nil, false, false},
		{"letsencrypt", []CAARecord{{Tag: "issue", Value: "letsencrypt.org"}}, true, true},
		{"with params", []CAARecord{{Tag: "issue", Value: "letsencrypt.org; validationmethods=http-01"}}, true, true},
		{"other ca", []CAARecord{{Tag: "issue", Value: "digicert.com"}}, false, true},
		{"iodef only", []CAARecord{{Tag: "iodef", Value: "mailto:x@example.com"}}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, p := caaAllows(tc.recs, "issue")
			if a != tc.allowed || p != tc.present {
				t.Errorf("caaAllows = %v,%v want %v,%v", a, p, tc.allowed, tc.present)
			}
		})
	}
}
