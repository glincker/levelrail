package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsinspect"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	envDNSLocalCacheFallback     = "APP_DNS_LOCAL_CACHE_FALLBACK"
	defaultDNSLocalCacheFallback = 5 * time.Minute
	defaultDNSWaitBudget         = 10 * time.Minute
)

func (rt *Router) dnsWaitBudget(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil {
		return envDurationOr("APP_DOMAINS_DNS_WAIT_BUDGET", defaultDNSWaitBudget)
	}
	return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyDomainsDNSWaitBudget)
}

func (rt *Router) dnsInspectorOrDefault() *dnsinspect.Inspector {
	if rt.dnsInspector != nil {
		return rt.dnsInspector
	}
	return dnsinspect.New()
}

// applyDNSVerdict adds the authoritative, public and local answers and lets
// the authoritative verdict decide the status, so this host's resolver cache
// never holds a correctly configured domain back.
func (rt *Router) applyDNSVerdict(ctx context.Context, resp *domainCheckResponse, domain string, expected []string, local domainCheckResult) {
	insp := rt.dnsInspectorOrDefault()
	if len(insp.Public) == 0 {
		return
	}
	host := wildcardProbeHost(domain)
	ictx, cancel := context.WithTimeout(ctx, 3*domainCheckLookupTimeout)
	defer cancel()
	view := insp.Inspect(ictx, host)
	localSrc := insp.Local(ictx, host)
	if len(localSrc.Addresses) == 0 && len(local.resolvedHosts) > 0 {
		// The resolv.conf probe failed; fall back to what the system lookup saw.
		localSrc = dnsinspect.Source{Name: dnsinspect.LocalName, Addresses: local.resolvedHosts}
	}
	now := time.Now()
	flush := dnsinspect.FlushCommand()
	if rt.dnsFlushCommand != nil {
		flush = rt.dnsFlushCommand()
	}
	in := domaindoctor.DNSVerdictInput{Expected: expected, Inspection: view, Local: localSrc, Now: now,
		Budget: rt.dnsWaitBudget(ctx), FallbackWait: envDurationOr(envDNSLocalCacheFallback, defaultDNSLocalCacheFallback), FlushCommand: flush}
	first := domaindoctor.JudgeDNS(in)
	unsettled := first.Verdict != domaindoctor.VerdictUnknown && (first.LocalStale || (first.Verdict != domaindoctor.VerdictReady && first.Verdict != domaindoctor.VerdictPublicReady))
	in.Waiting = rt.domainChecks.waited(domain, unsettled, now)
	v := domaindoctor.JudgeDNS(in)

	resp.Authoritative, resp.Public, resp.Local = view.Authoritative, view.Public, []dnsinspect.Source{localSrc}
	resp.Zone, resp.Verdict, resp.LocalStale, resp.Escalated, resp.Hints = view.Zone, v.Verdict, v.LocalStale, v.Escalated, v.Hints
	ns := view.ZoneNS
	if len(ns) == 0 {
		ns = view.DelegatedNS
	}
	resp.NameserverProvider = dnsinspect.ProviderOf(ns)
	if v.LocalStale {
		resp.FlushCommand = v.FlushCommand
	}
	if !v.RetryAt.IsZero() {
		resp.RetryAt = v.RetryAt.Format(time.RFC3339)
	}
	switch v.Verdict {
	case domaindoctor.VerdictReady:
		resp.Status = domainCheckStatusConnected
	case domaindoctor.VerdictPublicReady:
		resp.Status = domainCheckStatusPropagating
	case domaindoctor.VerdictElsewhere:
		resp.Status = domainCheckStatusResolvesElsewhere
	case domaindoctor.VerdictNotFound:
		resp.Status = domainCheckStatusNotResolving
	}
}
