package domaindoctor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"syscall"
)

// defaultCertMarkers maps a substring of a default certificate's subject or
// issuer to the proxy that serves it when it has no certificate for a host.
var defaultCertMarkers = []struct{ marker, proxy string }{
	{"TRAEFIK DEFAULT CERT", "Traefik"},
	{"Caddy Local Authority", "Caddy (internal issuer)"},
	{"Kubernetes Ingress Controller Fake Certificate", "ingress-nginx"},
	{"Plesk", "Plesk"},
	{"CN=localhost", "a proxy's self-signed default"},
}

// hstsPreloadedTLDs are whole TLDs in the browsers' HSTS preload list.
var hstsPreloadedTLDs = []string{"app", "bank", "boo", "dad", "day", "dev", "esq", "foo", "gle", "ing", "insurance", "meme", "mov", "new", "nexus", "page", "phd", "prof", "rsvp", "zip"}

func (rn *run) decideProbe(context.Context) Check {
	if len(rn.facts.ServerIPs) == 0 {
		rn.report.ProbeNote = "this server's public address is unknown, so the doctor did not connect to the domain; set APP_PUBLIC_HOST"
		return Check{}
	}
	here := intersect(rn.resolved, rn.facts.ServerIPs)
	if len(here) == 0 {
		rn.report.ProbeNote = "the domain does not resolve to this server, so the doctor only ran DNS checks; it never connects to addresses other than this server's own"
		return Check{}
	}
	rn.probeIP = here[0]
	for _, ip := range here {
		if p := net.ParseIP(ip); p != nil && p.To4() != nil {
			rn.probeIP = ip
			break
		}
	}
	rn.report.Probed = true
	return Check{}
}

func (rn *run) notProbed(c Check) Check {
	c.State = StateSkipped
	c.Detail = "not probed: " + rn.report.ProbeNote
	return c
}

func (rn *run) hostWithPort(host string, port, def int) string {
	if port == def {
		return host
	}
	return net.JoinHostPort(host, fmt.Sprint(port))
}

func (rn *run) checkPort80(ctx context.Context) Check {
	c := Check{ID: IDPort80, Title: "Port 80 answers", Tier: TierCertificate}
	if rn.probeIP == "" || rn.probes.HTTP == nil {
		return rn.notProbed(c)
	}
	pctx, cancel := rn.probeCtx(ctx)
	res, err := rn.probes.HTTP.Get(pctx, joinHostPort(rn.probeIP, rn.httpPort()), "http", rn.hostWithPort(rn.facts.Domain, rn.httpPort(), 80), "/")
	cancel()
	if err != nil {
		return connectFailure(c, fmt.Sprintf("port %d", rn.httpPort()), err,
			&Fix{Summary: "Open port 80 on this server and in any cloud firewall; Let's Encrypt validates over it."})
	}
	c.State = StatePass
	c.Detail = fmt.Sprintf("HTTP %d", res.Status)
	return c
}

func connectFailure(c Check, what string, err error, fix *Fix) Check {
	c.Fix = fix
	if errors.Is(err, syscall.ECONNREFUSED) {
		c.State = StateFail
		c.Detail = what + ": connection refused, nothing is listening"
		return c
	}
	c.State = StateWarn
	c.Detail = what + ": " + err.Error() + " (probed from this server; a NAT that does not hairpin can cause this even when visitors connect fine)"
	return c
}

func defaultCertProxy(pc PeerCert) string {
	for _, m := range defaultCertMarkers {
		if strings.Contains(pc.Subject, m.marker) || strings.Contains(pc.Issuer, m.marker) {
			return m.proxy
		}
	}
	return ""
}

func certCovers(names []string, host string) bool {
	for _, n := range names {
		n = strings.ToLower(n)
		if n == host {
			return true
		}
		if strings.HasPrefix(n, "*.") {
			if i := strings.IndexByte(host, '.'); i > 0 && host[i+1:] == n[2:] {
				return true
			}
		}
	}
	return false
}

func (rn *run) checkTLS(ctx context.Context) Check {
	c := Check{ID: IDTLS, Title: "HTTPS certificate is valid", Tier: TierCertificate}
	if rn.probeIP == "" || rn.probes.TLS == nil {
		return rn.notProbed(c)
	}
	pctx, cancel := rn.probeCtx(ctx)
	pc, err := rn.probes.TLS.Handshake(pctx, joinHostPort(rn.probeIP, rn.httpsPort()), rn.facts.Domain)
	cancel()
	if err != nil {
		rn.tlsErr = err
		c.Tier = TierTraffic
		return connectFailure(c, fmt.Sprintf("port %d", rn.httpsPort()), err,
			&Fix{Summary: fmt.Sprintf("Open port %d on this server and in any cloud firewall.", rn.httpsPort())})
	}
	rn.tls = &pc
	days := int(pc.NotAfter.Sub(rn.opts.Now()).Hours() / 24)
	if proxy := defaultCertProxy(pc); proxy != "" {
		c.State = StateFail
		c.Title = "A default certificate from " + proxy + " is served"
		c.Detail = fmt.Sprintf("subject %q, issuer %q: the proxy on port %d has no certificate for %s", pc.Subject, pc.Issuer, rn.httpsPort(), rn.facts.Domain)
		c.Fix = &Fix{Summary: "Give that proxy a route and certificate for this domain that forwards to this platform, or let this platform own ports 80 and 443.",
			Action: &Action{Kind: ActionRequest, Label: "Show proxy setup", API: "GET /api/v1/system/reverse-proxy?domain=" + rn.facts.Domain}}
		return c
	}
	switch {
	case !certCovers(pc.DNSNames, rn.facts.Domain):
		c.State = StateFail
		c.Title = "Certificate does not cover " + rn.facts.Domain
		c.Detail = "it is for " + strings.Join(pc.DNSNames, ", ")
		c.Fix = &Fix{Summary: "Something else answers on port 443 for this name; check which proxy owns it."}
	case days < 0:
		c.State, c.Title = StateFail, "Certificate served has expired"
		c.Detail = "issuer " + pc.Issuer
		c.Fix = &Fix{Summary: "Renew the certificate.", Action: rn.renewAction()}
	case !pc.Verified:
		c.State, c.Title = StateFail, "Certificate is not trusted by browsers"
		c.Detail = "issuer " + pc.Issuer + ": " + pc.VerifyError
		c.Fix = &Fix{Summary: "Turn on trusted certificates (Let's Encrypt) for this platform, or upload a certificate from a public CA."}
	case days < rn.opts.ExpiryWarnDays:
		c.State, c.Tier = StateWarn, TierSoon
		c.Title = fmt.Sprintf("Certificate served expires in %d days", days)
		c.Detail = "issuer " + pc.Issuer
		c.Fix = &Fix{Summary: "Renewal should happen on its own; renew now if it does not.", Action: rn.renewAction()}
	default:
		c.State = StatePass
		c.Detail = fmt.Sprintf("issuer %s, expires in %d days", pc.Issuer, days)
	}
	return c
}

func (rn *run) renewAction() *Action {
	if rn.facts.App == "" {
		return nil
	}
	return &Action{Kind: ActionRequest, Label: "Renew now", API: "POST " + rn.appDomainAPI("/cert/renew")}
}

func (rn *run) checkHTTPStatus(ctx context.Context) Check {
	c := Check{ID: IDHTTPStatus, Title: "Site answers over HTTPS", Tier: TierTraffic}
	if rn.probeIP == "" || rn.probes.HTTP == nil {
		return rn.notProbed(c)
	}
	if rn.tlsErr != nil {
		c.State, c.Detail = StateSkipped, "waiting on "+IDTLS
		return c
	}
	pctx, cancel := rn.probeCtx(ctx)
	res, err := rn.probes.HTTP.Get(pctx, joinHostPort(rn.probeIP, rn.httpsPort()), "https", rn.hostWithPort(rn.facts.Domain, rn.httpsPort(), 443), "/")
	cancel()
	if err != nil {
		c.State, c.Detail = StateWarn, err.Error()
		return c
	}
	rn.https = &res
	c.Detail = fmt.Sprintf("GET / returned %d", res.Status)
	switch {
	case res.Status == 502 || res.Status == 503 || res.Status == 504:
		c.State, c.Title = StateFail, fmt.Sprintf("The app behind %s is not answering (%d)", rn.facts.Domain, res.Status)
		c.Fix = &Fix{Summary: "Check that the app is running and listens on its configured port."}
	case res.Status >= 500:
		c.State, c.Title = StateFail, fmt.Sprintf("The site returns %d", res.Status)
		c.Fix = &Fix{Summary: "Check the app's logs for the error."}
	case res.Status == 404:
		c.State = StateWarn
		c.Fix = &Fix{Summary: "If your app serves /, the route for this domain may be missing."}
	default:
		c.State = StatePass
	}
	return c
}

func wwwVariant(host string) string {
	if strings.HasPrefix(host, "www.") {
		return strings.TrimPrefix(host, "www.")
	}
	return "www." + host
}

func (rn *run) checkRedirects(ctx context.Context) Check {
	c := Check{ID: IDRedirects, Title: "Redirects are sane", Tier: TierHygiene}
	if rn.probeIP == "" || rn.probes.HTTP == nil {
		return rn.notProbed(c)
	}
	cur, _ := url.Parse("http://" + rn.facts.Domain + "/")
	chain := []string{cur.String()}
	seen := map[string]bool{cur.String(): true}
	for hop := 0; hop <= rn.opts.MaxRedirects; hop++ {
		port := rn.httpPort()
		if cur.Scheme == "https" {
			port = rn.httpsPort()
		}
		pctx, cancel := rn.probeCtx(ctx)
		res, err := rn.probes.HTTP.Get(pctx, joinHostPort(rn.probeIP, port), cur.Scheme, cur.Host, cur.RequestURI())
		cancel()
		if err != nil {
			c.State, c.Detail = StateSkipped, "could not follow: "+err.Error()
			return c
		}
		if res.Status < 300 || res.Status >= 400 || res.Location == "" {
			return rn.judgeChain(c, chain)
		}
		next, err := cur.Parse(res.Location)
		if err != nil {
			c.State, c.Detail = StateWarn, "invalid Location header: "+res.Location
			return c
		}
		if seen[next.String()] {
			c.State, c.Tier = StateFail, TierTraffic
			c.Title = "Redirect loop"
			c.Detail = strings.Join(append(chain, next.String()), " > ")
			c.Fix = &Fix{Summary: "Usually a proxy in front forwards plain HTTP while the app or this platform forces HTTPS. Mark TLS as terminated upstream, or turn off one of the two HTTPS redirects."}
			return c
		}
		seen[next.String()] = true
		chain = append(chain, next.String())
		host := strings.ToLower(next.Hostname())
		if host != rn.facts.Domain && host != wwwVariant(rn.facts.Domain) {
			c.State = StatePass
			c.Detail = strings.Join(chain, " > ") + " (leaves this server, not followed)"
			return c
		}
		cur = next
	}
	c.State, c.Tier, c.Title = StateFail, TierTraffic, "Too many redirects"
	c.Detail = strings.Join(chain, " > ")
	return c
}

func (rn *run) judgeChain(c Check, chain []string) Check {
	c.Detail = strings.Join(chain, " > ")
	final := chain[len(chain)-1]
	switch {
	case !strings.HasPrefix(final, "https://") && rn.tls != nil && rn.tls.Verified:
		c.State = StateWarn
		c.Title = "HTTP does not redirect to HTTPS"
		c.Fix = &Fix{Summary: "Turn on Force HTTPS for this domain so visitors never stay on plain HTTP."}
	default:
		c.State = StatePass
	}
	if len(chain) > 1 {
		if u, err := url.Parse(final); err == nil && strings.ToLower(u.Hostname()) == wwwVariant(rn.facts.Domain) {
			c.Detail += " (www and apex redirect to each other)"
		}
	}
	return c
}

func (rn *run) preloadedTLD() string {
	i := strings.LastIndexByte(rn.facts.Domain, '.')
	if i < 0 {
		return ""
	}
	tld := rn.facts.Domain[i+1:]
	if slices.Contains(hstsPreloadedTLDs, tld) {
		return tld
	}
	return ""
}

func (rn *run) checkHSTS(context.Context) Check {
	c := Check{ID: IDHSTS, Title: "HSTS does not lock visitors out", Tier: TierCertificate}
	tld := rn.preloadedTLD()
	header := ""
	if rn.https != nil {
		header = rn.https.HSTS
	}
	if rn.probeIP == "" {
		c = rn.notProbed(c)
		if tld != "" {
			c.Detail += fmt.Sprintf("; note: every .%s domain is HSTS preloaded, so it only works with a trusted certificate", tld)
		}
		return c
	}
	tlsBad := rn.tlsErr == nil && rn.tls != nil && (!rn.tls.Verified || defaultCertProxy(*rn.tls) != "")
	var why []string
	if header != "" {
		why = append(why, "this site sends Strict-Transport-Security ("+header+")")
	}
	if tld != "" {
		why = append(why, "every ."+tld+" domain is in the browsers' HSTS preload list")
	}
	switch {
	case tlsBad && len(why) > 0:
		c.State = StateFail
		c.Title = "Browsers will refuse this site with no way to click through"
		c.Detail = "the certificate is not trusted and " + strings.Join(why, ", and ") +
			", so browsers show net::ERR_CERT_AUTHORITY_INVALID (or a similar HSTS error) and offer no bypass"
		c.Fix = &Fix{Summary: "Serve a trusted certificate for this domain first (see the HTTPS certificate check); HSTS cannot be undone from the server side for visitors who already saw it."}
	case strings.Contains(strings.ToLower(header), "preload"):
		c.State = StatePass
		c.Detail = "HSTS with preload: browsers will require HTTPS for this domain for months even if you remove it"
	default:
		c.State = StatePass
		if len(why) > 0 {
			c.Detail = strings.Join(why, "; ")
		}
	}
	return c
}
