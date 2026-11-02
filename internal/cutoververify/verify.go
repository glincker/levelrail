// Package cutoververify checks a domain that is being moved onto this server
// and answers with one verdict per aspect (DNS, proxy, redirect, certificate,
// HSTS, response, www/apex pair) plus the next step for each. It is read only:
// it looks names up and makes plain GET requests, nothing else.
package cutoververify

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"
)

// Verdict is the outcome of one line or of the whole report.
type Verdict string

// Verdict values, worst last.
const (
	Pass Verdict = "PASS"
	Warn Verdict = "WARN"
	Fail Verdict = "FAIL"
)

// Line ids, in report order.
const (
	LineDNS         = "dns"
	LineProxy       = "proxy"
	LineRedirect    = "redirect"
	LineCertificate = "certificate"
	LineHSTS        = "hsts"
	LineResponse    = "response"
	LinePair        = "pair"
)

// Resolver is the DNS surface the verifier needs.
type Resolver interface {
	LookupHost(ctx context.Context, name string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
}

// HTTPResult is one plain GET, redirects not followed.
type HTTPResult struct {
	Status   int
	Location string
	// Header holds the response headers with canonical keys.
	Header map[string]string
	TTFB   time.Duration
}

// CertInfo is what the served certificate showed.
type CertInfo struct {
	Issuer   string
	NotAfter time.Time
	Trusted  bool
}

// Prober makes the requests. Implementations must connect only to addrs, the
// set DNS answered once, while keeping the name for SNI and the Host header.
type Prober interface {
	HTTP(ctx context.Context, rawURL string, addrs []string) (HTTPResult, error)
	Cert(ctx context.Context, host string, addrs []string) (CertInfo, error)
}

// Deps are the seams tests replace.
type Deps struct {
	Resolver Resolver
	Prober   Prober
	// IsProxyIP reports whether an address belongs to a known proxy network.
	IsProxyIP func(ip string) bool
	Now       func() time.Time
}

// Options are the thresholds, read from env by the caller.
type Options struct {
	CertWarnDays int
	CertFailDays int
	// SlowTTFB is the time to first byte above which the response line warns.
	SlowTTFB time.Duration
	// HSTSMinAge is the smallest max-age (seconds) that counts as enabled.
	HSTSMinAge int
}

// Line is one verified aspect.
type Line struct {
	ID      string  `json:"id"`
	Verdict Verdict `json:"verdict"`
	Title   string  `json:"title"`
	Detail  string  `json:"detail"`
	Next    string  `json:"next"`
}

// Report is the whole answer for one domain.
type Report struct {
	Domain    string    `json:"domain"`
	Verdict   Verdict   `json:"verdict"`
	Summary   string    `json:"summary"`
	ServerIPs []string  `json:"server_ips"`
	Resolved  []string  `json:"resolved,omitempty"`
	CNAME     string    `json:"cname,omitempty"`
	Proxy     string    `json:"proxy,omitempty"`
	Lines     []Line    `json:"lines"`
	CheckedAt time.Time `json:"checked_at"`
}

// Verify runs every line for domain against serverIPs (this host's public addresses).
func Verify(ctx context.Context, d Deps, domain string, serverIPs []string, o Options) Report {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.IsProxyIP == nil {
		d.IsProxyIP = func(string) bool { return false }
	}
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	v := &verifier{d: d, o: o, domain: domain, server: serverIPs}
	rep := Report{Domain: domain, ServerIPs: serverIPs, CheckedAt: d.Now().UTC()}

	v.resolve(ctx)
	rep.Resolved, rep.CNAME = publicOnly(v.resolved), v.cname
	v.dnsLine()
	reachable := (v.pointsHere || v.proxiedIP) && !v.nonPublic
	if reachable {
		v.probe(ctx)
	}
	v.proxyLine()
	v.redirectLine(reachable)
	v.certLine(reachable)
	v.hstsLine(reachable)
	v.responseLine(reachable)
	v.pairLine(ctx)
	rep.Proxy = v.proxy
	rep.Lines = v.lines
	rep.Verdict = Pass
	fails, warns := 0, 0
	for _, l := range v.lines {
		switch l.Verdict {
		case Fail:
			fails++
			rep.Verdict = Fail
		case Warn:
			warns++
			if rep.Verdict == Pass {
				rep.Verdict = Warn
			}
		}
	}
	switch rep.Verdict {
	case Pass:
		rep.Summary = domain + " is ready: every check passed"
	case Warn:
		rep.Summary = fmt.Sprintf("%s works, %d %s to review", domain, warns, plural(warns, "item", "items"))
	default:
		rep.Summary = fmt.Sprintf("%s is not ready: %d %s to fix first", domain, fails, plural(fails, "item", "items"))
	}
	return rep
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

type verifier struct {
	d      Deps
	o      Options
	domain string
	server []string
	lines  []Line

	resolved   []string
	cname      string
	lookupErr  error
	pointsHere bool
	proxiedIP  bool
	// nonPublic is set when DNS answered with an address that must not be probed.
	nonPublic bool
	proxy     string

	httpRes, httpsRes HTTPResult
	httpErr, httpsErr error
	cert              CertInfo
	certErr           error
}

func (v *verifier) add(id string, verdict Verdict, title, detail, next string) {
	v.lines = append(v.lines, Line{ID: id, Verdict: verdict, Title: title, Detail: detail, Next: next})
}

func (v *verifier) resolve(ctx context.Context) {
	v.resolved, v.lookupErr = v.d.Resolver.LookupHost(ctx, v.domain)
	slices.Sort(v.resolved)
	if c, err := v.d.Resolver.LookupCNAME(ctx, v.domain); err == nil {
		c = strings.ToLower(strings.TrimSuffix(c, "."))
		if c != v.domain {
			v.cname = c
		}
	}
	if len(v.resolved) == 0 {
		return
	}
	v.nonPublic = slices.ContainsFunc(v.resolved, func(ip string) bool { return !IsPublicIP(ip) })
	v.pointsHere = !slices.ContainsFunc(v.resolved, func(ip string) bool { return !containsIP(v.server, ip) })
	v.proxiedIP = !slices.ContainsFunc(v.resolved, func(ip string) bool { return !v.d.IsProxyIP(ip) })
}

func containsIP(list []string, ip string) bool {
	p := net.ParseIP(ip)
	return slices.ContainsFunc(list, func(s string) bool {
		q := net.ParseIP(s)
		return q != nil && p != nil && q.Equal(p)
	})
}

func (v *verifier) dnsLine() {
	title := "DNS points at this server"
	switch {
	case v.nonPublic:
		v.add(LineDNS, Fail, title, "resolves to a private or reserved address, so nothing was requested", "Point "+v.domain+" at "+firstOr(v.server, "this server's public address")+".")
	case len(v.resolved) == 0:
		detail := "the name does not resolve"
		if v.lookupErr != nil {
			detail += " (" + classifyLookup(v.lookupErr) + ")"
		}
		v.add(LineDNS, Fail, title, detail, "Create an A record for "+v.domain+" pointing at "+firstOr(v.server, "this server's public address")+".")
	case v.pointsHere:
		v.add(LineDNS, Pass, title, "resolves to "+strings.Join(v.resolved, ", ")+viaCNAME(v.cname), "Nothing to do.")
	case v.proxiedIP:
		v.add(LineDNS, Warn, title, "resolves to proxy addresses ("+strings.Join(v.resolved, ", ")+"), so the origin cannot be read from DNS", "Confirm the proxy's origin is "+firstOr(v.server, "this server")+".")
	default:
		v.add(LineDNS, Fail, title, "resolves to "+strings.Join(v.resolved, ", ")+viaCNAME(v.cname)+", not "+strings.Join(v.server, ", "), "Change the record to "+firstOr(v.server, "this server's public address")+" and wait for the old TTL to pass.")
	}
}

func viaCNAME(c string) string {
	if c == "" {
		return ""
	}
	return " (CNAME " + c + ")"
}

func firstOr(list []string, def string) string {
	if len(list) == 0 {
		return def
	}
	return list[0]
}

// platformHeaders names the services whose response headers give them away.
var platformHeaders = []struct{ header, contains, name string }{
	{"Cf-Ray", "", "Cloudflare"},
	{"Server", "cloudflare", "Cloudflare"},
	{"X-Vercel-Id", "", "Vercel"},
	{"Server", "vercel", "Vercel"},
	{"X-Nf-Request-Id", "", "Netlify"},
	{"Server", "netlify", "Netlify"},
	{"X-Amz-Cf-Id", "", "CloudFront"},
	{"X-Served-By", "cache-", "Fastly"},
}

func detectPlatform(h map[string]string) string {
	for _, p := range platformHeaders {
		val, ok := h[p.header]
		if ok && strings.Contains(strings.ToLower(val), p.contains) {
			return p.name
		}
	}
	return ""
}

func (v *verifier) probe(ctx context.Context) {
	// One resolved set serves the "points here" check and every request below.
	addrs := slices.Clone(v.resolved)
	v.httpRes, v.httpErr = v.d.Prober.HTTP(ctx, "http://"+v.domain+"/", addrs)
	v.httpsRes, v.httpsErr = v.d.Prober.HTTP(ctx, "https://"+v.domain+"/", addrs)
	v.cert, v.certErr = v.d.Prober.Cert(ctx, v.domain, addrs)
}

func (v *verifier) proxyLine() {
	title := "No proxy in the way"
	name := ""
	if v.proxiedIP {
		name = "Cloudflare"
	}
	if name == "" && v.httpsErr == nil {
		name = detectPlatform(v.httpsRes.Header)
	}
	v.proxy = name
	if name == "" {
		v.add(LineProxy, Pass, title, "no proxy or CDN detected in DNS or response headers", "Nothing to do.")
		return
	}
	v.add(LineProxy, Warn, title, name+" answers for this name before it reaches this server", "That works. Keep the origin pointing here, and use SSL mode Full (strict) once the certificate is issued.")
}

func (v *verifier) skipped(id, title string) {
	v.add(id, Warn, title, "not checked: DNS does not point at this server yet, so nothing was requested", "Fix the DNS line first, then run the check again.")
}

func (v *verifier) redirectLine(reachable bool) {
	title := "HTTP redirects to HTTPS"
	if !reachable {
		v.skipped(LineRedirect, title)
		return
	}
	if v.httpErr != nil {
		v.add(LineRedirect, Warn, title, "port 80 did not answer ("+Classify(v.httpErr)+")", "Open port 80 so http links redirect and HTTP-01 certificates can renew.")
		return
	}
	r := v.httpRes
	loc, shown := safeLocation(r.Location)
	switch {
	case r.Status >= 300 && r.Status < 400 && !shown:
		v.add(LineRedirect, Warn, title, fmt.Sprintf("%d to an address that is not public, not shown", r.Status), "Make the redirect target a public https:// address.")
	case r.Status >= 300 && r.Status < 400 && strings.HasPrefix(strings.ToLower(loc), "https://"):
		v.add(LineRedirect, Pass, title, fmt.Sprintf("%d to %s", r.Status, loc), "Nothing to do.")
	case r.Status >= 300 && r.Status < 400:
		v.add(LineRedirect, Warn, title, fmt.Sprintf("%d to %s, which is not https", r.Status, loc), "Make the redirect target an https:// address.")
	default:
		v.add(LineRedirect, Warn, title, fmt.Sprintf("http answered %d without redirecting", r.Status), "Turn on the HTTPS redirect for this domain.")
	}
}

func (v *verifier) certLine(reachable bool) {
	title := "Certificate is valid"
	if !reachable {
		v.skipped(LineCertificate, title)
		return
	}
	if v.certErr != nil {
		v.add(LineCertificate, Fail, title, "no certificate was served ("+Classify(v.certErr)+")", "Request the certificate before switching DNS: levelrail-cli domains cutover "+v.domain+" --get-certificate.")
		return
	}
	days := int(v.cert.NotAfter.Sub(v.d.Now()).Hours() / 24)
	detail := fmt.Sprintf("issued by %s, %d days left", clean(v.cert.Issuer), days)
	switch {
	case !v.cert.Trusted:
		v.add(LineCertificate, Fail, title, detail+", not trusted for this name", "Wait for the real certificate to be issued, or fix the route so the right host answers.")
	case days <= v.o.CertFailDays:
		v.add(LineCertificate, Fail, title, detail, "Renew now: check the domain's certificate status and the ACME runbook.")
	case days <= v.o.CertWarnDays:
		v.add(LineCertificate, Warn, title, detail, "Renewal should have started; check the certificate status if this does not change.")
	default:
		v.add(LineCertificate, Pass, title, detail, "Nothing to do.")
	}
}

func (v *verifier) hstsLine(reachable bool) {
	title := "HSTS is set"
	if !reachable {
		v.skipped(LineHSTS, title)
		return
	}
	if v.httpsErr != nil {
		v.add(LineHSTS, Warn, title, "no HTTPS response to read headers from", "Fix the response line first.")
		return
	}
	h := v.httpsRes.Header["Strict-Transport-Security"]
	if h == "" {
		v.add(LineHSTS, Warn, title, "no Strict-Transport-Security header", "Once HTTPS is stable, turn on HSTS in the domain's headers policy.")
		return
	}
	age := hstsMaxAge(h)
	if age < v.o.HSTSMinAge {
		v.add(LineHSTS, Warn, title, fmt.Sprintf("max-age %d is shorter than %d", age, v.o.HSTSMinAge), "Raise max-age once you are sure HTTPS works everywhere.")
		return
	}
	v.add(LineHSTS, Pass, title, clean(h), "Nothing to do.")
}

func hstsMaxAge(h string) int {
	for _, part := range strings.Split(h, ";") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(k, "max-age") {
			n := 0
			for _, c := range strings.Trim(val, `"`) {
				if c < '0' || c > '9' {
					return 0
				}
				n = n*10 + int(c-'0')
			}
			return n
		}
	}
	return 0
}

func (v *verifier) responseLine(reachable bool) {
	title := "HTTPS answers quickly"
	if !reachable {
		v.skipped(LineResponse, title)
		return
	}
	if v.httpsErr != nil {
		v.add(LineResponse, Fail, title, "https did not answer ("+Classify(v.httpsErr)+")", "Check that the app is running and the route exists: levelrail-cli domains doctor.")
		return
	}
	if p := detectPlatform(v.httpsRes.Header); p != "" && v.proxy == "" {
		v.proxy = p
	}
	r := v.httpsRes
	detail := fmt.Sprintf("status %d, first byte in %d ms", r.Status, r.TTFB.Milliseconds())
	switch {
	case r.Status >= 500:
		v.add(LineResponse, Fail, title, detail, "Read the app's logs; the server answers but the app errors.")
	case r.Status >= 400:
		v.add(LineResponse, Warn, title, detail, "Check that the app serves / for this host, or that the error is expected.")
	case v.o.SlowTTFB > 0 && r.TTFB > v.o.SlowTTFB:
		v.add(LineResponse, Warn, title, detail+" (slow)", "Check app startup and resource use; compare with the old host.")
	default:
		v.add(LineResponse, Pass, title, detail, "Nothing to do.")
	}
}

// pairLine compares the www and apex names, which people forget to move together.
func (v *verifier) pairLine(ctx context.Context) {
	title := "www and apex move together"
	other, ok := counterpart(v.domain)
	if !ok {
		v.add(LinePair, Pass, title, "not applicable to this name", "Nothing to do.")
		return
	}
	got, err := v.d.Resolver.LookupHost(ctx, other)
	if err != nil || len(got) == 0 {
		v.add(LinePair, Warn, title, other+" does not resolve", "Add a record for "+other+" (or a redirect) so visitors using it are not lost.")
		return
	}
	slices.Sort(got)
	switch {
	case !slices.Equal(got, v.resolved):
		v.add(LinePair, Warn, title, other+" resolves to "+describeAddrs(got)+", not the same as "+v.domain, "Point "+other+" at the same place.")
	default:
		v.add(LinePair, Pass, title, other+" resolves to the same addresses", "Nothing to do.")
	}
}

// publicOnly drops addresses that must not be shown or probed, so a report
// never reveals what a private or internal name resolves to.
func publicOnly(ips []string) []string {
	var out []string
	for _, ip := range ips {
		if IsPublicIP(ip) {
			out = append(out, ip)
		}
	}
	return out
}

// describeAddrs lists the public addresses and masks the rest.
func describeAddrs(ips []string) string {
	pub := publicOnly(ips)
	if len(pub) == len(ips) {
		return strings.Join(ips, ", ")
	}
	if len(pub) == 0 {
		return "a private or reserved address"
	}
	return strings.Join(pub, ", ") + " and a private or reserved address"
}

// counterpart maps www.x to x and a two-label name to www.x.
func counterpart(domain string) (string, bool) {
	if rest, ok := strings.CutPrefix(domain, "www."); ok && strings.Contains(rest, ".") {
		return rest, true
	}
	if strings.Count(domain, ".") == 1 {
		return "www." + domain, true
	}
	return "", false
}
