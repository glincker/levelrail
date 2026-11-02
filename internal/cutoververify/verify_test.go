package cutoververify

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"syscall"
	"testing"
	"time"
)

type fakeResolver struct {
	hosts map[string][]string
	cname map[string]string
	// seq, when set, answers each call for a name with the next entry.
	seq   map[string][][]string
	calls map[string]int
}

func (f fakeResolver) LookupHost(_ context.Context, n string) ([]string, error) {
	if q, ok := f.seq[n]; ok {
		i := f.calls[n]
		f.calls[n]++
		return q[min(i, len(q)-1)], nil
	}
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
	addrs   [][]string
}

func (f *fakeProber) HTTP(_ context.Context, u string, addrs []string) (HTTPResult, error) {
	f.calls++
	f.addrs = append(f.addrs, addrs)
	if err := f.httpErr[u]; err != nil {
		return HTTPResult{}, err
	}
	return f.http[u], nil
}

func (f *fakeProber) Cert(_ context.Context, _ string, addrs []string) (CertInfo, error) {
	f.calls++
	f.addrs = append(f.addrs, addrs)
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
	got, err := NetProber{Timeout: 2 * time.Second, allowNonPublic: true}.HTTP(context.Background(), srv.URL, []string{"127.0.0.1"})
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

func TestIsPublicIP(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "10.0.0.5", "172.16.0.1", "192.168.1.1", "fd00::1", "fe80::1", "169.254.169.254", "0.0.0.0", "::", "224.0.0.1", "ff02::1", "100.100.100.200", "::ffff:127.0.0.1", "fd00:ec2::254", "nope"} {
		if IsPublicIP(ip) {
			t.Errorf("%s must not be public", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "203.0.113.10", "2606:4700::1111"} {
		if !IsPublicIP(ip) {
			t.Errorf("%s must be public", ip)
		}
	}
}

// DNS answers a public address once and a loopback address afterwards: the
// verifier must look the name up once and hand that same set to every probe.
func TestVerify_RebindingResolverUsedOnce(t *testing.T) {
	res := fakeResolver{
		seq:   map[string][][]string{"example.com": {{"203.0.113.10"}, {"127.0.0.1"}}},
		hosts: map[string][]string{"www.example.com": {"203.0.113.10"}},
		calls: map[string]int{},
	}
	p := healthyProber()
	p.http = map[string]HTTPResult{
		"http://example.com/":  {Status: 301, Location: "https://example.com/"},
		"https://example.com/": {Status: 200, Header: map[string]string{"Strict-Transport-Security": "max-age=31536000"}},
	}
	Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
	if res.calls["example.com"] != 1 {
		t.Errorf("name looked up %d times, want 1", res.calls["example.com"])
	}
	if len(p.addrs) != 3 {
		t.Fatalf("probe calls = %d", len(p.addrs))
	}
	for _, a := range p.addrs {
		if len(a) != 1 || a[0] != "203.0.113.10" {
			t.Errorf("probe got addrs %v, want the resolved public set", a)
		}
	}
}

func TestVerify_PrivateAnswerIsNeverProbed(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"10.0.0.7"}}}
	p := healthyProber()
	rep := Verify(context.Background(), deps(res, p), "example.com", []string{"10.0.0.7"}, opts)
	if p.calls != 0 {
		t.Errorf("prober called %d times for a private address", p.calls)
	}
	if l := line(t, rep, LineDNS); l.Verdict != Fail || !strings.Contains(l.Detail, "private or reserved") {
		t.Errorf("dns line = %+v", l)
	}
}

// The probe dials the pinned address even though the URL name is not it, and
// refuses a non-public pinned address outright.
func TestNetProber_DialStaysPinned(t *testing.T) {
	var dialed []string
	p := NetProber{Timeout: time.Second, dial: func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed = append(dialed, addr)
		return nil, syscall.ECONNREFUSED
	}}
	_, err := p.HTTP(context.Background(), "https://rebind.example.com/", []string{"203.0.113.10"})
	if err == nil || len(dialed) != 1 || dialed[0] != "203.0.113.10:443" {
		t.Fatalf("dialed %v err %v", dialed, err)
	}
	dialed = nil
	_, err = p.HTTP(context.Background(), "https://rebind.example.com/", []string{"203.0.113.10", "127.0.0.1"})
	if !errors.Is(err, ErrNonPublicAddress) || len(dialed) != 0 {
		t.Fatalf("mixed set: dialed %v err %v", dialed, err)
	}
	if _, err = (NetProber{Timeout: time.Second}).Cert(context.Background(), "rebind.example.com", []string{"127.0.0.1"}); Classify(err) != ClassBlocked {
		t.Errorf("cert to loopback: %v", err)
	}
}

// The socket-level check refuses a loopback target even if the pre-check were bypassed.
func TestNetProber_ControlRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	ap := netip.MustParseAddrPort(srv.Listener.Addr().String())
	p := NetProber{Timeout: time.Second}
	d := net.Dialer{Control: p.control}
	if c, err := d.Dial("tcp", ap.String()); err == nil {
		_ = c.Close()
		t.Fatal("dial to loopback succeeded")
	} else if !errors.Is(err, ErrNonPublicAddress) {
		t.Fatalf("err = %v", err)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"dns":     {&net.DNSError{Err: "no such host", Name: "x"}, ClassDNSFailed},
		"refused": {&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, ClassConnectRefused},
		"timeout": {context.DeadlineExceeded, ClassTimeout},
		"tls":     {errors.New("remote error: tls: handshake failure"), ClassTLSFailed},
		"blocked": {ErrNonPublicAddress, ClassBlocked},
		"dial":    {&net.OpError{Op: "dial", Err: errors.New("no route to host 10.1.2.3")}, ClassConnectFailed},
		"other":   {errors.New("boom 10.1.2.3"), ClassRequestFailed},
	}
	for name, c := range cases {
		if got := Classify(c.err); got != c.want {
			t.Errorf("%s: Classify = %q, want %q", name, got, c.want)
		}
	}
}

func TestVerify_ErrorsShowClassesNotRawText(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"203.0.113.10"}, "www.example.com": {"203.0.113.10"}}}
	p := healthyProber()
	p.httpErr = map[string]error{
		"http://example.com/":  &net.OpError{Op: "dial", Err: errors.New("dial tcp 10.9.9.9:80: refused"), Addr: &net.TCPAddr{IP: net.ParseIP("10.9.9.9")}},
		"https://example.com/": &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
	}
	p.certErr = errors.New("tls: internal error at 10.9.9.9")
	rep := Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
	for _, id := range []string{LineRedirect, LineCertificate, LineResponse} {
		if d := line(t, rep, id).Detail; strings.Contains(d, "10.9.9.9") {
			t.Errorf("%s leaks raw error: %q", id, d)
		}
	}
	if d := line(t, rep, LineResponse).Detail; !strings.Contains(d, ClassConnectRefused) {
		t.Errorf("response detail = %q", d)
	}
	if d := line(t, rep, LineCertificate).Detail; !strings.Contains(d, ClassTLSFailed) {
		t.Errorf("certificate detail = %q", d)
	}
	rep = Verify(context.Background(), deps(fakeResolver{}, healthyProber()), "gone.example.com", []string{"203.0.113.10"}, opts)
	if d := line(t, rep, LineDNS).Detail; !strings.Contains(d, ClassDNSFailed) || strings.Contains(d, "no such host") {
		t.Errorf("dns detail = %q", d)
	}
}

func TestVerify_NonPublicRedirectTargetIsHidden(t *testing.T) {
	res := fakeResolver{hosts: map[string][]string{"example.com": {"203.0.113.10"}, "www.example.com": {"203.0.113.10"}}}
	for _, loc := range []string{"http://169.254.169.254/latest/meta-data", "https://localhost/x", "https://db.internal/", "http://2130706433/", "http://127.1/"} {
		p := healthyProber()
		p.http = map[string]HTTPResult{
			"http://example.com/":  {Status: 302, Location: loc},
			"https://example.com/": {Status: 200},
		}
		rep := Verify(context.Background(), deps(res, p), "example.com", []string{"203.0.113.10"}, opts)
		d := line(t, rep, LineRedirect)
		if d.Verdict != Warn || strings.Contains(d.Detail, "169.254") || strings.Contains(d.Detail, "internal") || strings.Contains(d.Detail, "localhost") || strings.Contains(d.Detail, "2130706433") || strings.Contains(d.Detail, "127.1") {
			t.Errorf("location %q shown: %+v", loc, d)
		}
	}
	if loc, ok := safeLocation("/login"); !ok || loc != "/login" {
		t.Errorf("relative location = %q %v", loc, ok)
	}
}
