package domaindoctor

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Check IDs, stable for the dashboard and alert prefills.
const (
	IDDNSResolves    = "dns.resolves"
	IDDNSPointsHere  = "dns.points_here"
	IDDNSAAAA        = "dns.aaaa_mismatch"
	IDDNSApexCNAME   = "dns.cname_apex"
	IDCAA            = "caa.blocks_issuer"
	IDAppRunning     = "app.running"
	IDHeldBack       = "domain.held_back"
	IDProxyPorts     = "proxy.ports"
	IDPort80         = "net.port80"
	IDTLS            = "tls.certificate"
	IDHTTPStatus     = "http.status"
	IDRedirects      = "redirect.chain"
	IDHSTS           = "hsts.risk"
	IDStoredCert     = "cert.renewal"
	letsEncryptIssue = "letsencrypt.org"
)

func (rn *run) checkResolves(ctx context.Context) Check {
	c := Check{ID: IDDNSResolves, Title: "DNS resolves", Tier: TierTraffic}
	if rn.probes.DNS == nil {
		c.State, c.Detail = StateUnavailable, "no DNS probe configured"
		return c
	}
	pctx, cancel := context.WithTimeout(ctx, 2*rn.opts.ProbeTimeout)
	rn.answers = rn.probes.DNS.Answers(pctx, rn.facts.Domain)
	cancel()

	answered, matching := 0, 0
	var parts []string
	for _, a := range rn.answers {
		addrs := append(slices.Clone(a.IPv4), a.IPv6...)
		for _, ip := range addrs {
			if !slices.Contains(rn.resolved, ip) {
				rn.resolved = append(rn.resolved, ip)
			}
		}
		switch {
		case len(addrs) > 0:
			answered++
			if overlaps(addrs, rn.facts.ServerIPs) {
				matching++
			}
			parts = append(parts, a.Resolver+": "+strings.Join(addrs, ", "))
		case a.Error != "":
			parts = append(parts, a.Resolver+": "+a.Error)
		default:
			parts = append(parts, a.Resolver+": no answer")
		}
	}
	c.Detail = strings.Join(parts, "; ")
	switch {
	case answered == 0:
		c.State = StateFail
		c.Title = rn.facts.Domain + " does not resolve"
		c.Fix = &Fix{Summary: fmt.Sprintf("Add an A record for %s pointing at %s at your DNS provider.", rn.facts.Domain, firstOr(rn.facts.ServerIPs, "this server's public address")),
			Action: &Action{Kind: ActionCopy, Label: "Copy record", Value: fmt.Sprintf("%s A %s", rn.facts.Domain, firstOr(rn.facts.ServerIPs, ""))}}
	case answered < len(rn.answers) || (matching > 0 && matching < answered):
		c.State = StateWarn
		c.Title = "DNS is propagating"
		c.Fix = &Fix{Summary: "Resolvers disagree. Wait for the old answer's TTL to pass, then check again."}
	default:
		c.State = StatePass
	}
	return c
}

func (rn *run) checkPointsHere(context.Context) Check {
	c := Check{ID: IDDNSPointsHere, Title: "DNS points to this server", Tier: TierTraffic}
	switch {
	case len(rn.resolved) == 0:
		c.State, c.Detail = StateSkipped, "waiting on "+IDDNSResolves
	case len(rn.facts.ServerIPs) == 0:
		c.State, c.Detail = StateUnavailable, "this server's public address is unknown; set APP_PUBLIC_HOST"
	case overlaps(rn.resolved, rn.facts.ServerIPs):
		c.State, c.Detail = StatePass, "resolves to "+strings.Join(intersect(rn.resolved, rn.facts.ServerIPs), ", ")
	case rn.facts.TLSTerminatedUpstream:
		c.State = StateWarn
		c.Detail = fmt.Sprintf("resolves to %s, not this server (%s). Expected when your own proxy or CDN fronts this server.", strings.Join(rn.resolved, ", "), strings.Join(rn.facts.ServerIPs, ", "))
		c.Fix = &Fix{Summary: "Make sure your proxy forwards this domain to this server."}
	default:
		c.State = StateFail
		c.Title = rn.facts.Domain + " points somewhere else"
		c.Detail = fmt.Sprintf("resolves to %s, this server is %s", strings.Join(rn.resolved, ", "), strings.Join(rn.facts.ServerIPs, ", "))
		c.Fix = &Fix{Summary: fmt.Sprintf("Change the A record for %s to %s.", rn.facts.Domain, rn.facts.ServerIPs[0]),
			Action: &Action{Kind: ActionCopy, Label: "Copy record", Value: fmt.Sprintf("%s A %s", rn.facts.Domain, rn.facts.ServerIPs[0])}}
	}
	return c
}

func (rn *run) checkAAAA(context.Context) Check {
	c := Check{ID: IDDNSAAAA, Title: "IPv6 (AAAA) records match", Tier: TierTraffic}
	var v6 []string
	for _, a := range rn.answers {
		for _, ip := range a.IPv6 {
			if !slices.Contains(v6, ip) {
				v6 = append(v6, ip)
			}
		}
	}
	serverV6 := ipv6Only(rn.facts.ServerIPs)
	switch {
	case len(v6) == 0:
		c.State, c.Detail = StatePass, "no AAAA records"
	case rn.facts.TLSTerminatedUpstream:
		c.State, c.Detail = StateSkipped, "a proxy in front owns the public addresses"
	case overlaps(v6, serverV6):
		c.State = StatePass
	case len(serverV6) == 0:
		c.State = StateFail
		c.Detail = fmt.Sprintf("AAAA records (%s) exist but this server has no public IPv6 address; IPv6 visitors go elsewhere", strings.Join(v6, ", "))
		c.Fix = &Fix{Summary: "Remove the AAAA records for " + rn.facts.Domain + "."}
	default:
		c.State = StateFail
		c.Detail = fmt.Sprintf("AAAA points to %s, this server's IPv6 is %s", strings.Join(v6, ", "), strings.Join(serverV6, ", "))
		c.Fix = &Fix{Summary: fmt.Sprintf("Change the AAAA record to %s or remove it.", serverV6[0])}
	}
	return c
}

func (rn *run) checkApexCNAME(ctx context.Context) Check {
	c := Check{ID: IDDNSApexCNAME, Title: "No CNAME at the zone apex", Tier: TierHygiene}
	apex, err := publicsuffix.EffectiveTLDPlusOne(rn.facts.Domain)
	if err != nil || apex != rn.facts.Domain {
		c.State, c.Detail = StateSkipped, "not a zone apex"
		return c
	}
	if rn.probes.DNS == nil {
		c.State = StateUnavailable
		return c
	}
	pctx, cancel := rn.probeCtx(ctx)
	target, err := rn.probes.DNS.CNAME(pctx, rn.facts.Domain)
	cancel()
	switch {
	case err != nil:
		c.State, c.Detail = StateUnavailable, "CNAME lookup failed"
	case target != "":
		c.State = StateWarn
		c.Detail = "the apex has a CNAME to " + target + "; most providers reject this and it breaks MX and TXT records"
		c.Fix = &Fix{Summary: "Use an A record, or your provider's ALIAS or CNAME flattening, at the apex."}
	default:
		c.State = StatePass
	}
	return c
}

func (rn *run) checkCAA(ctx context.Context) Check {
	c := Check{ID: IDCAA, Title: "CAA allows Let's Encrypt", Tier: TierCertificate}
	switch {
	case rn.facts.TLSTerminatedUpstream:
		c.State, c.Detail = StateSkipped, "TLS is terminated by your own proxy"
		return c
	case !rn.facts.ACMEEnabled:
		c.State, c.Detail = StateSkipped, "trusted certificates are off on this platform"
		return c
	case rn.probes.DNS == nil:
		c.State = StateUnavailable
		return c
	}
	pctx, cancel := rn.probeCtx(ctx)
	recs, err := rn.probes.DNS.CAA(pctx, rn.facts.Domain)
	cancel()
	if err != nil {
		c.State, c.Detail = StateUnavailable, "CAA lookup failed"
		return c
	}
	if len(recs) == 0 {
		c.State, c.Detail = StatePass, "no CAA records, any CA may issue"
		return c
	}
	tag := "issue"
	if strings.HasPrefix(rn.facts.Domain, "*.") {
		tag = "issuewild"
	}
	allowed, present := caaAllows(recs, tag)
	if !present && tag == "issuewild" {
		allowed, present = caaAllows(recs, "issue")
	}
	if !present || allowed {
		c.State = StatePass
		return c
	}
	c.State = StateFail
	c.Title = "CAA records block Let's Encrypt"
	c.Detail = "CAA at " + recs[0].Name + " allows only: " + caaIssuers(recs, tag)
	c.Fix = &Fix{Summary: fmt.Sprintf("Add a CAA record at %s: 0 %s \"%s\".", recs[0].Name, tag, letsEncryptIssue),
		Action: &Action{Kind: ActionCopy, Label: "Copy record", Value: fmt.Sprintf("%s CAA 0 %s \"%s\"", recs[0].Name, tag, letsEncryptIssue)}}
	return c
}

// caaAllows reports whether recs permit Let's Encrypt for tag, and whether
// any record with that tag exists at all.
func caaAllows(recs []CAARecord, tag string) (allowed, present bool) {
	for _, r := range recs {
		if r.Tag != tag {
			continue
		}
		present = true
		issuer := strings.TrimSpace(strings.SplitN(r.Value, ";", 2)[0])
		if strings.EqualFold(issuer, letsEncryptIssue) {
			allowed = true
		}
	}
	return allowed, present
}

func caaIssuers(recs []CAARecord, tag string) string {
	var out []string
	for _, r := range recs {
		if r.Tag == tag {
			v := strings.TrimSpace(strings.SplitN(r.Value, ";", 2)[0])
			if v == "" {
				v = "(nobody)"
			}
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}

func overlaps(a, b []string) bool {
	return len(intersect(a, b)) > 0
}

func intersect(a, b []string) []string {
	var out []string
	for _, x := range a {
		if slices.Contains(b, x) && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func ipv6Only(ips []string) []string {
	var out []string
	for _, s := range ips {
		if ip := net.ParseIP(s); ip != nil && ip.To4() == nil {
			out = append(out, s)
		}
	}
	return out
}

func firstOr(list []string, fallback string) string {
	if len(list) > 0 {
		return list[0]
	}
	return fallback
}
