package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

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

func defaultLookupHost(ctx context.Context, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
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
)

// domainCheckResponse is GET
// /api/v1/apps/{name}/domains/{domain}/check's wire shape: what
// DomainEditor needs to tell an operator the exact DNS record to add and
// whether it has actually taken effect, without either side needing to
// know DNS terminology beyond "A record."
type domainCheckResponse struct {
	Domain string `json:"domain"`
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
func (rt *Router) runDomainCheck(ctx context.Context, domain, expectedHost string, inferred bool) domainCheckResponse {
	resp := domainCheckResponse{Domain: domain, ExpectedHost: expectedHost, HostInferred: inferred}
	if expectedHost == "" {
		resp.Status = domainCheckStatusUnconfigured
		return resp
	}

	result, cached := rt.domainChecks.get(domain)
	if !cached {
		lookupCtx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		hosts, err := rt.lookupHost(lookupCtx, domain)
		cancel()
		result = domainCheckResult{resolvedHosts: hosts, resolved: err == nil && len(hosts) > 0, at: time.Now()}
		rt.domainChecks.set(domain, result)
	}

	resp.ResolvedHosts = result.resolvedHosts
	resp.Resolved = result.resolved
	if !result.resolved {
		resp.Status = domainCheckStatusNotResolving
		return resp
	}

	// Cached the same way and under the same TTL as the domain lookup
	// above, just keyed with a prefix no real domain can collide with
	// (a bare hostname is never dot-free the way "expected:" itself
	// is malformed as a domain). Without this, a hostname
	// APP_PUBLIC_HOST (as opposed to an IP literal, which
	// expectedIPs never calls lookupHost for at all) got one real,
	// uncached DNS lookup on every single poll, defeating the whole
	// point of the cache above for exactly the deployments where it
	// matters most (a fronted/load-balanced control plane).
	expectedCacheKey := "expected:" + expectedHost
	expectedResult, expectedCached := rt.domainChecks.get(expectedCacheKey)
	var expected []string
	if expectedCached {
		expected = expectedResult.resolvedHosts
	} else {
		lookupCtx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		expected = expectedIPs(lookupCtx, rt.lookupHost, expectedHost)
		cancel()
		rt.domainChecks.set(expectedCacheKey, domainCheckResult{resolvedHosts: expected, at: time.Now()})
	}

	if hostsOverlap(result.resolvedHosts, expected) {
		resp.Status = domainCheckStatusConnected
	} else {
		resp.Status = domainCheckStatusResolvesElsewhere
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
	resp := rt.runDomainCheck(r.Context(), domain, expectedHost, inferred)
	writeJSON(w, http.StatusOK, resp)
}
