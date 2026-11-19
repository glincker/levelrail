package api

import (
	"context"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

// Longest hint the checklist shows, so a long doctor detail stays one line.
const siteDiagnosisHintMax = 140

// siteDiagnosis is one line of the Pages domain checklist.
type siteDiagnosis struct {
	Code string `json:"code"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint"`
}

// Doctor checks a Pages domain surfaces, in display order.
var siteDiagnosisChecks = []string{
	domaindoctor.IDDNSResolves, domaindoctor.IDDNSPointsHere, domaindoctor.IDDNSAAAA,
	domaindoctor.IDDNSApexCNAME, domaindoctor.IDDNSProxied, domaindoctor.IDProxyPorts,
	domaindoctor.IDPort80, domaindoctor.IDCAA, domaindoctor.IDTLS, domaindoctor.IDStoredCert,
	domaindoctor.IDHTTPStatus,
}

const (
	diagCodePropagation = "dns.propagation"
	diagCodePrivate     = "host.private_address"
	diagCodeACME        = "acme.failure"
	diagCodeWWW         = "dns.www_apex"
)

func shortHint(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > siteDiagnosisHintMax {
		return strings.TrimSpace(string(r[:siteDiagnosisHintMax-3])) + "..."
	}
	return s
}

// diagnosisFromReport maps doctor checks to checklist rows; checks that did
// not run (skipped, unavailable) are left out rather than shown as passing.
func diagnosisFromReport(rep domaindoctor.Report) []siteDiagnosis {
	out := []siteDiagnosis{}
	for _, id := range siteDiagnosisChecks {
		i := slices.IndexFunc(rep.Checks, func(c domaindoctor.Check) bool { return c.ID == id })
		if i < 0 {
			continue
		}
		c := rep.Checks[i]
		if c.State == domaindoctor.StateSkipped || c.State == domaindoctor.StateUnavailable {
			continue
		}
		ok := c.State == domaindoctor.StatePass
		hint := c.Title
		switch {
		case !ok && c.Fix != nil && c.Fix.Summary != "":
			hint = c.Fix.Summary
		case !ok && c.Cause != "":
			hint = c.Cause
		case c.Detail != "":
			hint = c.Detail
		}
		out = append(out, siteDiagnosis{Code: c.ID, OK: ok, Hint: shortHint(hint)})
	}
	if p := rep.Propagation; p != nil {
		out = append(out, siteDiagnosis{Code: diagCodePropagation, OK: p.State == domaindoctor.PropagationSettled, Hint: shortHint(p.Detail)})
	}
	return out
}

// extraDiagnosis adds the rows the doctor has no check for: a private or
// CGNAT host address, the recorded ACME failure and a www or apex mismatch.
func (rt *Router) extraDiagnosis(ctx context.Context, domain string) []siteDiagnosis {
	var out []siteDiagnosis
	check := rt.runDomainCheckOpts(ctx, domain, rt.advertisedPublicHost(ctx), false, false)
	rt.enrichDomainCheck(ctx, &check)
	if check.ExpectedHost != "" {
		hint := "this host has a public address"
		if check.ExpectedPrivate {
			hint = "this host sits on a private or CGNAT address; use DNS-01 or a tunnel"
		}
		out = append(out, siteDiagnosis{Code: diagCodePrivate, OK: !check.ExpectedPrivate, Hint: hint})
	}
	if a := acmeFailureIn(rt.tlsSnapshot(ctx), domain); a != nil {
		out = append(out, siteDiagnosis{Code: diagCodeACME, OK: false, Hint: shortHint(domaindoctor.ExplainACME(a.Error))})
	} else {
		out = append(out, siteDiagnosis{Code: diagCodeACME, OK: true, Hint: "no certificate failure recorded"})
	}
	if d, ok := rt.wwwApexDiagnosis(ctx, domain, check.ResolvedHosts); ok {
		out = append(out, d)
	}
	return out
}

// wwwApexDiagnosis compares an apex or www host with its sibling; other
// subdomains have no sibling worth checking.
func (rt *Router) wwwApexDiagnosis(ctx context.Context, domain string, resolved []string) (siteDiagnosis, bool) {
	if !strings.HasPrefix(domain, "www.") && strings.Count(domain, ".") != 1 {
		return siteDiagnosis{}, false
	}
	var sibling string
	if strings.HasPrefix(domain, "www.") {
		sibling = strings.TrimPrefix(domain, "www.")
	} else {
		sibling = "www." + domain
	}
	lctx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
	defer cancel()
	hosts, err := rt.lookupHost(lctx, sibling)
	d := siteDiagnosis{Code: diagCodeWWW}
	switch {
	case err != nil || len(hosts) == 0:
		// Optional: many sites serve only one of the two names.
		d.OK, d.Hint = true, sibling+" has no DNS record (optional)"
	case len(resolved) > 0 && !hostsOverlap(hosts, resolved):
		d.Hint = sibling + " points at a different address"
	default:
		d.OK, d.Hint = true, sibling+" resolves to the same host"
	}
	return d, true
}

// siteDomainDiagnosis returns the cached diagnosis for one site domain, or a
// fresh run when force is set or nothing is cached yet.
func (rt *Router) siteDomainDiagnosis(ctx context.Context, name, domain string, force bool) []siteDiagnosis {
	ttl := envDuration(envPagesDoctorCacheTTL, defaultPagesDoctorCacheTTL)
	return rt.siteDiag.get(ctx, name+"\x00"+domain, force, ttl, func(c context.Context) []siteDiagnosis {
		return rt.runSiteDomainDiagnosis(c, name, domain)
	})
}

// runSiteDomainDiagnosis runs the domain doctor and the extra rows for one site.
func (rt *Router) runSiteDomainDiagnosis(ctx context.Context, name, domain string) []siteDiagnosis {
	rep, status, _ := rt.runDomainDoctor(ctx, name, domain, nil)
	if status != 0 {
		return []siteDiagnosis{}
	}
	return append(diagnosisFromReport(rep), rt.extraDiagnosis(ctx, domain)...)
}
