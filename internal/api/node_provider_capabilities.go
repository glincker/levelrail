package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	defaultCapCacheTTLSeconds = 900
	defaultCapRecheckMinutes  = 360
	defaultCapRefreshGapSecs  = 60
	defaultCapProbeParallel   = 4
	capProbeOverallTimeout    = 40 * time.Second
	errCodeProviderScope      = "provider_scope_missing"
)

// capLearned is a write permission result observed from a real call.
type capLearned struct {
	ok bool
	at time.Time
}

type capEntry struct {
	probeMu     sync.Mutex // serialises probes so concurrent readers share one
	result      *provision.ProviderCapabilities
	lastRefresh time.Time
	learned     map[string]capLearned
}

// providerCapCache is in memory: learned write states reset on restart.
type providerCapCache struct {
	mu            sync.Mutex
	entries       map[string]*capEntry
	recheckActive atomic.Bool
	resources     providerReadCache[resourceSnapshot]
	spend         providerReadCache[provision.SpendReport]
}

func (c *providerCapCache) entry(provider string) *capEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*capEntry{}
	}
	e := c.entries[provider]
	if e == nil {
		e = &capEntry{learned: map[string]capLearned{}}
		c.entries[provider] = e
	}
	return e
}

// invalidate drops cached and learned state when the credential changes.
func (c *providerCapCache) invalidate(provider string) {
	c.mu.Lock()
	delete(c.entries, provider)
	c.mu.Unlock()
	c.resources.invalidate(provider)
	c.spend.invalidate(provider)
}

// recordProviderCapabilityLearned stores the outcome of a real provider write
// for a capability: success is learned_ok, a 401 or 403 is learned_403.
func (rt *Router) recordProviderCapabilityLearned(provider, capabilityID string, err error) {
	ok := err == nil
	if !ok {
		if _, scope := provision.ScopeFromError(provider, capabilityID, err); !scope {
			return
		}
	}
	e := rt.providerCaps.entry(provider)
	e.probeMu.Lock()
	e.learned[capabilityID] = capLearned{ok: ok, at: time.Now().UTC()}
	e.probeMu.Unlock()
}

// writeProviderScopeMissing turns a provider 401 or 403 into a fixed
// "needs scope X" answer without echoing the provider body.
func (rt *Router) writeProviderScopeMissing(w http.ResponseWriter, provider, capabilityID string, err error) bool {
	se, ok := provision.ScopeFromError(provider, capabilityID, err)
	if !ok {
		return false
	}
	rt.recordProviderCapabilityLearned(provider, capabilityID, err)
	msg := "The " + provider + " token needs the " + se.Needs + " scope for this action."
	writeJSON(w, http.StatusFailedDependency, map[string]string{
		"error": msg, "code": errCodeProviderScope, "capability": capabilityID, "needs": se.Needs,
		"request_id": w.Header().Get(requestIDHeader),
	})
	return true
}

func (rt *Router) capSetting(ctx context.Context, key, env string, def int) int {
	return rt.tunableInt(ctx, key, env, def)
}

func (rt *Router) capRecheckInterval(ctx context.Context) time.Duration {
	var n int
	if rt.runtimeSettings != nil {
		n = rt.runtimeSettings.Int(ctx, runtimesettings.KeyProvisionCapRecheckMin)
	} else {
		n = envIntDefault("APP_PROVISION_CAPABILITIES_RECHECK_INTERVAL_MINUTES", defaultCapRecheckMinutes)
	}
	if n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Minute
}

// overlayLearned applies learned write results to unknown capabilities only,
// so a fresh probe result is never overridden by an older write.
func overlayLearned(res *provision.ProviderCapabilities, learned map[string]capLearned) {
	for i := range res.Capabilities {
		c := &res.Capabilities[i]
		l, ok := learned[c.ID]
		if !ok || c.State != provision.CapabilityUnknown || c.Evidence != provision.EvidenceNotProbed {
			continue
		}
		c.ObservedAt = l.at
		if l.ok {
			c.State, c.Evidence = provision.CapabilityGranted, provision.EvidenceLearnedOK
		} else {
			c.State, c.Evidence = provision.CapabilityMissing, provision.Evidence403Learned
		}
	}
}

// capMode says how a caller wants capabilities: from cache, as a user
// refresh bound by the cooldown, or a forced probe after a credential change.
type capMode int

const (
	capServeCached capMode = iota
	capUserRefresh
	capForceProbe
)

// providerCapabilities serves cached capabilities or runs one probe. The
// per-provider probeMu keeps concurrent callers to a single probe. A
// non-zero retryAfter means the refresh cooldown refused the call.
func (rt *Router) providerCapabilities(ctx context.Context, provider string, p NodeProvisioner, mode capMode) (out provision.ProviderCapabilities, retryAfter time.Duration) {
	e := rt.providerCaps.entry(provider)
	e.probeMu.Lock()
	defer e.probeMu.Unlock()
	return rt.providerCapabilitiesLocked(ctx, provider, p, e, mode)
}

// providerCapabilitiesLocked runs with e.probeMu held by the caller.
func (rt *Router) providerCapabilitiesLocked(ctx context.Context, provider string, p NodeProvisioner, e *capEntry, mode capMode) (provision.ProviderCapabilities, time.Duration) {
	ttl := time.Duration(rt.capSetting(ctx, runtimesettings.KeyProvisionCapCacheTTL, "APP_PROVISION_CAPABILITIES_CACHE_TTL_SECONDS", defaultCapCacheTTLSeconds)) * time.Second
	gap := time.Duration(rt.capSetting(ctx, runtimesettings.KeyProvisionCapRefreshMinGap, "APP_PROVISION_CAPABILITIES_REFRESH_MIN_INTERVAL_SECONDS", defaultCapRefreshGapSecs)) * time.Second
	parallel := rt.capSetting(ctx, runtimesettings.KeyProvisionCapProbeParallel, "APP_PROVISION_CAPABILITIES_PROBE_CONCURRENCY", defaultCapProbeParallel)
	now := time.Now()
	if mode == capUserRefresh && !e.lastRefresh.IsZero() && now.Sub(e.lastRefresh) < gap {
		return provision.ProviderCapabilities{}, gap - now.Sub(e.lastRefresh)
	}
	fresh := e.result != nil && now.Sub(e.result.CheckedAt) < ttl
	cached := fresh && mode == capServeCached
	if !cached {
		var res provision.ProviderCapabilities
		if prober, ok := p.(provision.CapabilityProber); ok {
			pctx, cancel := context.WithTimeout(ctx, capProbeOverallTimeout)
			res = prober.ProbeCapabilities(pctx, parallel)
			cancel()
			if res.LiveCheckError != "" {
				rt.logger.Warn("api: provider capability probe incomplete", slog.String("provider", provider))
			}
		} else {
			res = provision.ProviderCapabilities{Provider: provider, AccountID: provider + "-default", Method: "none", CheckedAt: now.UTC(), Capabilities: []provision.Capability{}}
		}
		if res.CheckedAt.IsZero() {
			res.CheckedAt = now.UTC()
		}
		if mode != capServeCached {
			e.lastRefresh = now
		}
		e.result = &res
	}
	out := *e.result
	out.Capabilities = append([]provision.Capability{}, out.Capabilities...)
	overlayLearned(&out, e.learned)
	out.Cached = cached
	if iv := rt.capRecheckInterval(ctx); iv > 0 {
		next := out.CheckedAt.Add(iv)
		out.NextCheckAt = &next
	}
	return out, 0
}

// handleGetNodeProviderCapabilities handles GET
// /api/v1/node-providers/{provider}/capabilities?refresh=1: granted, missing
// or unknown per capability, from read-only probes, cached. Always 200 with a
// partial result when the provider is unreachable.
func (rt *Router) handleGetNodeProviderCapabilities(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == nodeProviderCustom {
		writeError(w, http.StatusBadRequest, "a manual data center has no cloud API to check")
		return
	}
	p, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), provider)
	if !ok {
		writeError(w, status, msg)
		return
	}
	mode := capServeCached
	if r.URL.Query().Get("refresh") == "1" {
		mode = capUserRefresh
	}
	out, retry := rt.providerCapabilities(r.Context(), provider, p, mode)
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "provider permissions were just rechecked, try again shortly")
		return
	}
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	writeJSON(w, http.StatusOK, out)
}
