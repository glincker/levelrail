package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// domainCheckLookupTimeout bounds a single DNS resolution this handler
// performs, so a slow or blackholed resolver can never turn GET
// .../domains/{domain}/check into a request DomainEditor's "Check now"
// button waits on forever.
const domainCheckLookupTimeout = 4 * time.Second

// domainCheckCacheTTL is how long a domain's resolved-hosts result is
// reused before a fresh DNS query runs again. Short enough that "Check
// now" still feels live, long enough that DomainEditor's own bounded
// auto-poll (web/src/queries/domainCheck.ts) can't turn one open tab
// into a steady stream of outbound DNS queries against the same domain.
const domainCheckCacheTTL = 5 * time.Second

// lookupHostFunc resolves host to its A/AAAA addresses. Matches
// net.Resolver.LookupHost's own signature so the real implementation is
// a one-line wrapper; overridable per-Router the same way fetchFunc and
// listBranchesFunc already are, so tests never perform a real DNS query.
type lookupHostFunc func(ctx context.Context, host string) ([]string, error)

// publicResolvers are asked when the system resolver finds nothing, because a
// recursive resolver caches "does not exist" for the zone's whole negative TTL
// (often 30 minutes) after a record is added, so the local answer lags the DNS
// provider the operator just edited.
var publicResolvers = []string{"1.1.1.1:53", "8.8.8.8:53"}

func lookupVia(ctx context.Context, server, host string) ([]string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, server)
		},
	}
	return r.LookupHost(ctx, host)
}

func defaultLookupHost(ctx context.Context, host string) ([]string, error) {
	hosts, err := net.DefaultResolver.LookupHost(ctx, host)
	if err == nil && len(hosts) > 0 {
		return hosts, nil
	}
	for _, server := range publicResolvers {
		if got, perr := lookupVia(ctx, server, host); perr == nil && len(got) > 0 {
			return got, nil
		}
	}
	return hosts, err
}

// domainCheckResult is one lookup outcome, cached by domain.
type domainCheckResult struct {
	resolvedHosts []string
	resolved      bool
	at            time.Time
}

// domainCheckCache rate-limits actual DNS lookups per domain, the same
// in-memory-map-plus-mutex shape loginLimiter (ratelimit.go) already
// uses for a different per-key throttling problem. A control plane
// restart clearing it is an accepted tradeoff, not a regression worth a
// persistent store for: the next request simply does one fresh lookup.
type domainCheckCache struct {
	mu      sync.Mutex
	results map[string]domainCheckResult
}

func newDomainCheckCache() *domainCheckCache {
	return &domainCheckCache{results: make(map[string]domainCheckResult)}
}

func (c *domainCheckCache) get(domain string) (domainCheckResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	res, ok := c.results[domain]
	if !ok || time.Since(res.at) > domainCheckCacheTTL {
		return domainCheckResult{}, false
	}
	return res, true
}

func (c *domainCheckCache) set(domain string, res domainCheckResult) {
	c.mu.Lock()
	c.results[domain] = res
	c.mu.Unlock()
}

// Domain check status values: the four states DomainEditor's
// DomainDnsCheck component renders.
const (
	domainCheckStatusConnected         = "connected"
	domainCheckStatusNotResolving      = "not_resolving"
	domainCheckStatusResolvesElsewhere = "resolves_elsewhere"
	domainCheckStatusUnconfigured      = "unconfigured"
	// domainCheckStatusPropagating means public DNS already answers with this
	// server but the server's own resolver has not caught up (negative caching).
	domainCheckStatusPropagating = "propagating"
)

// domainCheckResponse is GET
// /api/v1/apps/{name}/domains/{domain}/check's wire shape: what
// DomainEditor needs to tell an operator the exact DNS record to add and
// whether it has actually taken effect, without either side needing to
// know DNS terminology beyond "A record."
type domainCheckResponse struct {
	Domain string `json:"domain"`
	// CheckedAt is when this answer was produced (RFC 3339), so a click on
	// "Check now" visibly changes something.
	CheckedAt string `json:"checked_at,omitempty"`
	// Resolvers lists what this server and public resolvers answered, set
	// when the domain is not yet connected.
	Resolvers []resolverResult `json:"resolvers,omitempty"`
	// ExpectedHost is the IP or hostname an A/CNAME record for Domain
	// should point at to reach this control plane's embedded ingress:
	// rt.publicHost (APP_PUBLIC_HOST) when configured, else a best-effort
	// guess from the request's own Host header (see advertisedHost).
	// Empty only when neither is available.
	ExpectedHost string `json:"expected_host,omitempty"`
	// HostInferred is true when ExpectedHost came from the request's Host
	// header rather than APP_PUBLIC_HOST, so the frontend can say "best
	// guess, configure APP_PUBLIC_HOST for accuracy" instead of stating it
	// as fact.
	HostInferred  bool     `json:"host_inferred,omitempty"`
	Resolved      bool     `json:"resolved"`
	ResolvedHosts []string `json:"resolved_hosts,omitempty"`
	Status        string   `json:"status"`
	// ExpectedIPv4/ExpectedIPv6 split ExpectedHost's resolved addresses
	// by family, so the frontend can offer A/AAAA as alternatives to
	// CNAME when ExpectedHost is a hostname with both.
	ExpectedIPv4 []string `json:"expected_ipv4,omitempty"`
	ExpectedIPv6 []string `json:"expected_ipv6,omitempty"`
	// ExpectedPrivate is true when the address Domain must point at is a
	// private/LAN one, so no public CA can validate it over HTTP-01.
	ExpectedPrivate bool `json:"expected_private,omitempty"`
	// Challenge is "http-01" or "dns-01-required".
	Challenge string `json:"challenge,omitempty"`
	// DNSProvider is the active DNS-01 provider: cloudflare, route53, none.
	DNSProvider string `json:"dns_provider,omitempty"`
	// ACMEFailure is the CA's last error for Domain, when one is recorded.
	ACMEFailure *acmeFailureResource `json:"acme_failure,omitempty"`
}

// advertisedHost picks the host DomainEditor should tell an operator to
// point their DNS record at: configured (APP_PUBLIC_HOST, via
// WithPublicHost) if set, since this control plane cannot reliably
// discover its own public-facing address on its own (NAT, a container
// port mapping, a cloud load balancer all break self-discovery); else a
// best-effort guess stripped from the request's own Host header, since
// an operator reaching the dashboard at some address is very likely
// reaching it on the same host their apps' ingress serves from too, in
// this project's single-binary architecture.
func advertisedHost(r *http.Request, configured string) (host string, inferred bool) {
	if configured != "" {
		return configured, false
	}
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		return h, true
	}
	return r.Host, true
}

// expectedIPs resolves host into the IP set a domain's own resolved
// addresses need to overlap with to count as "connected": host itself,
// if it's already an IP literal (the expected case for an A record
// target), or host's own resolved addresses otherwise, so an operator
// can point APP_PUBLIC_HOST at a stable hostname instead of a raw IP and
// the check still works.
func expectedIPs(ctx context.Context, lookup lookupHostFunc, host string) []string {
	if ip := net.ParseIP(host); ip != nil {
		return []string{host}
	}
	hosts, err := lookup(ctx, host)
	if err != nil {
		return nil
	}
	return hosts
}

// splitByFamily buckets addrs into IPv4 and IPv6.
func splitByFamily(addrs []string) (v4, v6 []string) {
	for _, a := range addrs {
		ip := net.ParseIP(a)
		switch {
		case ip == nil:
		case ip.To4() != nil:
			v4 = append(v4, a)
		default:
			v6 = append(v6, a)
		}
	}
	return v4, v6
}

func hostsOverlap(a, b []string) bool {
	set := make(map[string]struct{}, len(b))
	for _, h := range b {
		set[h] = struct{}{}
	}
	for _, h := range a {
		if _, ok := set[h]; ok {
			return true
		}
	}
	return false
}

// runDomainCheck resolves domain and compares it against expectedHost,
// the shared logic behind both handleCheckDomain (per-app) and
// handleCheckIngressDomain (the platform's own primary domain, in
// ingress_settings.go): same cache, same lookupHost, same status rules,
// so the two endpoints can never quietly drift apart on what "connected"
// means.
// detectedPublicIPs probes this host's own public addresses, cached so a
// polling dashboard does not hammer the probe services.
func (rt *Router) detectedPublicIPs(ctx context.Context) []string {
	const key = "detected-public-ips"
	if cached, ok := rt.domainChecks.get(key); ok {
		return cached.resolvedHosts
	}
	detect := rt.detectPublicIPs
	if detect == nil {
		detect = ingress.DetectPublicIPs
	}
	ips := detect(ctx)
	rt.domainChecks.set(key, domainCheckResult{resolvedHosts: ips, at: time.Now()})
	return ips
}

func (rt *Router) runDomainCheck(ctx context.Context, domain, expectedHost string, inferred bool) domainCheckResponse {
	return rt.runDomainCheckOpts(ctx, domain, expectedHost, inferred, false)
}

// resolverResult is what one resolver answered for the domain, so the
// dashboard can show why a check says what it says.
type resolverResult struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// queryPublicResolvers asks well-known public resolvers directly, bypassing
// the local stub and its negative cache.
func queryPublicResolvers(ctx context.Context, host string) []resolverResult {
	if v := strings.ToLower(os.Getenv("APP_DNS_PUBLIC_RESOLVERS")); v == "off" || v == "false" || v == "0" {
		return nil
	}
	out := make([]resolverResult, 0, len(publicResolvers))
	for _, server := range publicResolvers {
		got, err := lookupVia(ctx, server, host)
		rr := resolverResult{Name: strings.TrimSuffix(server, ":53"), Addresses: got}
		if err != nil {
			rr.Error = "no answer"
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
				rr.Error = "not found"
			}
		}
		out = append(out, rr)
	}
	return out
}

// runDomainCheckOpts is runDomainCheck with refresh, which skips the short
// result cache so a manual "Check now" always asks DNS again.
func (rt *Router) runDomainCheckOpts(ctx context.Context, domain, expectedHost string, inferred, refresh bool) domainCheckResponse {
	resp := domainCheckResponse{Domain: domain, ExpectedHost: expectedHost, HostInferred: inferred, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if expectedHost == "" {
		resp.Status = domainCheckStatusUnconfigured
		return resp
	}

	result, cached := rt.domainChecks.get(domain)
	if !cached || refresh {
		lookupCtx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		hosts, err := rt.lookupHost(lookupCtx, domain)
		cancel()
		result = domainCheckResult{resolvedHosts: hosts, resolved: err == nil && len(hosts) > 0, at: time.Now()}
		rt.domainChecks.set(domain, result)
	}
	resp.ResolvedHosts = result.resolvedHosts
	resp.Resolved = result.resolved

	// Cached under a prefix no real domain can collide with, so a hostname
	// APP_PUBLIC_HOST is not looked up again on every poll.
	expectedCacheKey := "expected:" + expectedHost
	expectedResult, expectedCached := rt.domainChecks.get(expectedCacheKey)
	var expected []string
	if expectedCached && !refresh {
		expected = expectedResult.resolvedHosts
	} else {
		lookupCtx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		expected = expectedIPs(lookupCtx, rt.lookupHost, expectedHost)
		cancel()
		// A dual-stack host answers on both families, so a domain pointed
		// at either is pointed at this server, not just at the configured one.
		for _, ip := range rt.detectedPublicIPs(ctx) {
			if !containsString(expected, ip) {
				expected = append(expected, ip)
			}
		}
		rt.domainChecks.set(expectedCacheKey, domainCheckResult{resolvedHosts: expected, at: time.Now()})
	}
	resp.ExpectedIPv4, resp.ExpectedIPv6 = splitByFamily(expected)

	switch {
	case result.resolved && hostsOverlap(result.resolvedHosts, expected):
		resp.Status = domainCheckStatusConnected
		return resp
	case result.resolved:
		resp.Status = domainCheckStatusResolvesElsewhere
	default:
		resp.Status = domainCheckStatusNotResolving
	}

	// Not connected from this server's point of view: say what this server's
	// resolver and public resolvers each answered, and report "propagating"
	// when the public answer is already right.
	local := resolverResult{Name: "this server", Addresses: result.resolvedHosts}
	if !result.resolved {
		local.Error = "not found"
	}
	resp.Resolvers = append(resp.Resolvers, local)
	lookup := rt.publicLookup
	if lookup == nil {
		lookup = queryPublicResolvers
	}
	pubCtx, cancel := context.WithTimeout(ctx, 2*domainCheckLookupTimeout)
	defer cancel()
	for _, rr := range lookup(pubCtx, domain) {
		resp.Resolvers = append(resp.Resolvers, rr)
		if hostsOverlap(rr.Addresses, expected) {
			resp.Status = domainCheckStatusPropagating
		}
	}
	return resp
}

// handleCheckDomain handles GET
// /api/v1/apps/{name}/domains/{domain}/check: a real DNS lookup
// (net.LookupHost, no external dependency) for domain, reporting whether
// it currently resolves to this control plane's own advertised address.
// AbilityRead, the same passive-visibility tier as GET
// /api/v1/apps/{name}/git-source: this reads live DNS state, it performs
// no write.
func (rt *Router) handleCheckDomain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))

	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.logger.Error("api: check domain: get app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}

	expectedHost, inferred := advertisedHost(r, rt.publicHost)
	resp := rt.runDomainCheckOpts(r.Context(), domain, expectedHost, inferred, r.URL.Query().Get("refresh") == "true")
	rt.enrichDomainCheck(r.Context(), &resp)
	writeJSON(w, http.StatusOK, resp)
}

// CheckDomainStatus exposes runDomainCheck to internal/alerting's
// kind=domain_health evaluator (see alerting.DomainCheckSource), the same
// DNS check handleCheckDomain itself runs. Uses rt.publicHost only: unlike
// an HTTP handler, an engine tick has no request Host header to fall back
// on for advertisedHost's own best-effort guess, so a domain_health rule
// needs APP_PUBLIC_HOST configured to evaluate meaningfully; without it
// every domain simply reports domainCheckStatusUnconfigured, which
// EvaluateDomainHealth treats as inconclusive, not unhealthy.
func (rt *Router) CheckDomainStatus(ctx context.Context, domain string) (string, error) {
	return rt.runDomainCheck(ctx, domain, rt.publicHost, false).Status, nil
}
