package cutoververify

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeResolver struct {
	hosts map[string][]string
	cname map[string]string
}

func (f fakeResolver) LookupHost(_ context.Context, n string) ([]string, error) {
	if h, ok := f.hosts[n]; ok {
		return h, nil
	}
	return nil, errors.New("no such host")
}

func (f fakeResolver) LookupCNAME(_ context.Context, n string) (string, error) {
	if c, ok := f.cname[n]; ok {
		return c, nil
	}
	return n + ".", nil
}

type fakeProber struct {
	http    map[string]HTTPResult
	httpErr map[string]error
	cert    CertInfo
	certErr error
	calls   int
}

func (f *fakeProber) HTTP(_ context.Context, u string) (HTTPResult, error) {
	f.calls++
	if err := f.httpErr[u]; err != nil {
		return HTTPResult{}, err
	}
	return f.http[u], nil
}

func (f *fakeProber) Cert(context.Context, string) (CertInfo, error) {
	f.calls++
	return f.cert, f.certErr
}

var (
	now  = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	opts = Options{CertWarnDays: 21, CertFailDays: 7, SlowTTFB: time.Second, HSTSMinAge: 15552000}
)

func healthyProber() *fakeProber {
	return &fakeProber{
		http: map[string]HTTPResult{
			"http://shop.example.com/":  {Status: 301, Location: "https://shop.example.com/"},
			"https://shop.example.com/": {Status: 200, TTFB: 120 * time.Millisecond, Header: map[string]string{"Strict-Transport-Security": "max-age=31536000"}},
		},
		cert: CertInfo{Issuer: "Let's Encrypt R11", NotAfter: now.Add(60 * 24 * time.Hour), Trusted: true},
	}
}

func deps(r fakeResolver, p *fakeProber) Deps {
	return Deps{Resolver: r, Prober: p, Now: func() time.Time { return now }, IsProxyIP: func(ip string) bool { return strings.HasPrefix(ip, "104.") }}
}

func line(t *testing.T, r Report, id string) Line {
	t.Helper()
	for _, l := range r.Lines {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("no line %q", id)
	return Line{}
}

func TestVerify_AllPass(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"shop.example.com": {"203.0.113.10"}, "www.shop.example.com": {"203.0.113.10"}}}
	// www.shop has three labels, so the pair line is not applicable; use a two-label name below.
	rep := Verify(context.Background(), deps(res, healthyProber()), "shop.example.com", []string{"203.0.113.10"}, opts)
	if rep.Verdict != Pass {
		t.Fatalf("verdict = %s, lines = %+v", rep.Verdict, rep.Lines)
	}
	for _, l := range rep.Lines {
		if l.Next == "" {
			t.Errorf("line %s has no next step", l.ID)
		}
	}
}

func TestVerify_StillAtOldHostFailsAndSendsNoRequests(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"76.76.21.21"}}}
	p := healthyProber()
	rep := Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
	if rep.Verdict != Fail {
		t.Fatalf("verdict = %s", rep.Verdict)
	}
	if got := line(t, rep, LineDNS); got.Verdict != Fail || !strings.Contains(got.Next, "203.0.113.10") {
		t.Errorf("dns line = %+v", got)
	}
	if p.calls != 0 {
		t.Errorf("prober called %d times for a name that does not point here", p.calls)
	}
	if got := line(t, rep, LinePair); got.Verdict != Warn {
		t.Errorf("pair line = %+v, want WARN for unresolved www", got)
	}
}

func TestVerify_CloudflareProxy(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"104.21.1.1"}, "www.example.com": {"104.21.1.1"}}}
	p := healthyProber()
	p.http = map[string]HTTPResult{
		"http://example.com/":  {Status: 301, Location: "https://example.com/"},
		"https://example.com/": {Status: 200, TTFB: 50 * time.Millisecond, Header: map[string]string{"Cf-Ray": "abc", "Strict-Transport-Security": "max-age=31536000"}},
	}
	rep := Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
	if rep.Proxy != "Cloudflare" || line(t, rep, LineProxy).Verdict != Warn || line(t, rep, LineDNS).Verdict != Warn {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Verdict != Warn {
		t.Errorf("verdict = %s, want WARN", rep.Verdict)
	}
}

func TestVerify_CertAndResponseThresholds(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"203.0.113.10"}, "www.example.com": {"203.0.113.10"}}}
	cases := []struct {
		name   string
		mut    func(*fakeProber)
		id     string
		expect Verdict
	}{
		{"cert 10 days", func(p *fakeProber) { p.cert.NotAfter = now.Add(10 * 24 * time.Hour) }, LineCertificate, Warn},
		{"cert 3 days", func(p *fakeProber) { p.cert.NotAfter = now.Add(3 * 24 * time.Hour) }, LineCertificate, Fail},
		{"cert untrusted", func(p *fakeProber) { p.cert.Trusted = false }, LineCertificate, Fail},
		{"cert missing", func(p *fakeProber) { p.certErr = errors.New("handshake failure") }, LineCertificate, Fail},
		{"no redirect", func(p *fakeProber) { p.http["http://example.com/"] = HTTPResult{Status: 200} }, LineRedirect, Warn},
		{"port 80 closed", func(p *fakeProber) { p.httpErr = map[string]error{"http://example.com/": errors.New("refused")} }, LineRedirect, Warn},
		{"no hsts", func(p *fakeProber) { p.http["https://example.com/"] = HTTPResult{Status: 200} }, LineHSTS, Warn},
		{"short hsts", func(p *fakeProber) {
			p.http["https://example.com/"] = HTTPResult{Status: 200, Header: map[string]string{"Strict-Transport-Security": "max-age=300"}}
		}, LineHSTS, Warn},
		{"500", func(p *fakeProber) { p.http["https://example.com/"] = HTTPResult{Status: 502} }, LineResponse, Fail},
		{"404", func(p *fakeProber) { p.http["https://example.com/"] = HTTPResult{Status: 404} }, LineResponse, Warn},
		{"slow", func(p *fakeProber) { p.http["https://example.com/"] = HTTPResult{Status: 200, TTFB: 3 * time.Second} }, LineResponse, Warn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := healthyProber()
			p.http = map[string]HTTPResult{
				"http://example.com/":  {Status: 301, Location: "https://example.com/"},
				"https://example.com/": {Status: 200, Header: map[string]string{"Strict-Transport-Security": "max-age=31536000"}},
			}
			c.mut(p)
			rep := Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
			if got := line(t, rep, c.id); got.Verdict != c.expect {
				t.Errorf("%s = %+v, want %s", c.id, got, c.expect)
			}
		})
	}
}

func TestVerify_PairMismatch(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"203.0.113.10"}, "www.example.com": {"76.76.21.21"}}}
	p := healthyProber()
	p.http = map[string]HTTPResult{
		"http://example.com/":  {Status: 301, Location: "https://example.com/"},
		"https://example.com/": {Status: 200, Header: map[string]string{"Strict-Transport-Security": "max-age=31536000"}},
	}
	rep := Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
	if got := line(t, rep, LinePair); got.Verdict != Warn || !strings.Contains(got.Detail, "76.76.21.21") {
		t.Errorf("pair = %+v", got)
	}
}

func TestNetProber_HTTPDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://example.com/")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer srv.Close()
	got, err := NetProber{Timeout: 2 * time.Second}.HTTP(context.Background(), srv.URL)
	if err != nil || got.Status != 301 || got.Location != "https://example.com/" || got.TTFB <= 0 {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestHSTSMaxAge(t *testing.T) {
	for in, want := range map[string]int{"max-age=31536000; includeSubDomains": 31536000, "includeSubDomains": 0, "max-age=abc": 0, `max-age="600"`: 600} {
		if got := hstsMaxAge(in); got != want {
			t.Errorf("hstsMaxAge(%q) = %d, want %d", in, got, want)
		}
	}
}
