package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	defaultResourcesCacheTTLSecs = 60
	defaultResourcesRefreshGap   = 15
	defaultSpendCacheTTLSecs     = 3600
	defaultSpendRefreshGap       = 300
	providerReadTimeout          = 30 * time.Second
	managedTag                   = "levelrail"
	managedProvisionTagPrefix    = "levelrail-provision-"
)

// cacheSlot holds one provider's cached read. mu is held across the fetch so
// concurrent callers share a single provider call.
type cacheSlot[T any] struct {
	mu          sync.Mutex
	val         *T
	fetchedAt   time.Time
	lastRefresh time.Time
}

// providerReadCache is a per-provider TTL cache with a forced refresh
// cooldown. Zero value is usable.
type providerReadCache[T any] struct {
	mu    sync.Mutex
	slots map[string]*cacheSlot[T]
}

func (c *providerReadCache[T]) slot(provider string) *cacheSlot[T] {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.slots == nil {
		c.slots = map[string]*cacheSlot[T]{}
	}
	s := c.slots[provider]
	if s == nil {
		s = &cacheSlot[T]{}
		c.slots[provider] = s
	}
	return s
}

func (c *providerReadCache[T]) invalidate(provider string) {
	c.mu.Lock()
	delete(c.slots, provider)
	for k := range c.slots {
		if strings.HasPrefix(k, provider+"@") {
			delete(c.slots, k)
		}
	}
	c.mu.Unlock()
}

// get serves the cache inside ttl, or fetches. A forced refresh inside gap
// returns retryAfter. A failed fetch keeps serving the last good value
// unless the error is a missing scope, which must be shown.
func (c *providerReadCache[T]) get(provider string, ttl, gap time.Duration, refresh bool, fetch func() (T, error)) (val T, cached bool, retryAfter time.Duration, err error) {
	s := c.slot(provider)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if refresh && !s.lastRefresh.IsZero() && now.Sub(s.lastRefresh) < gap {
		return val, false, gap - now.Sub(s.lastRefresh), nil
	}
	if s.val != nil && !refresh && now.Sub(s.fetchedAt) < ttl {
		return *s.val, true, 0, nil
	}
	if refresh {
		s.lastRefresh = now
	}
	v, ferr := fetch()
	if ferr != nil {
		var se *provision.ScopeMissingError
		if s.val != nil && !errors.As(ferr, &se) && !isProviderAuthError(ferr) {
			return *s.val, true, 0, nil
		}
		s.val = nil
		return val, false, 0, ferr
	}
	s.val, s.fetchedAt = &v, now
	return v, false, 0, nil
}

func isProviderAuthError(err error) bool {
	var pe *provision.ProviderError
	return errors.As(err, &pe) && (pe.Status == http.StatusForbidden || pe.Status == http.StatusUnauthorized)
}

type providerResourcesResponse struct {
	Provider    string                       `json:"provider"`
	AccountID   string                       `json:"account_id"`
	GeneratedAt time.Time                    `json:"generated_at"`
	Cached      bool                         `json:"cached"`
	Count       int                          `json:"count"`
	Resources   []provision.ProviderResource `json:"resources"`
}

type resourceSnapshot struct {
	resources []provision.ProviderResource
	at        time.Time
}

// loadProviderResources serves the cached machine list or fetches it.
func (rt *Router) loadProviderResources(ctx context.Context, provider string, p NodeProvisioner, refresh bool) (snap resourceSnapshot, cached bool, retry time.Duration, err error) {
	return rt.loadProviderResourcesIn(ctx, provider, "", p, refresh)
}

// loadProviderResourcesIn is loadProviderResources for one region, cached in
// its own slot so regions never serve each other's list.
func (rt *Router) loadProviderResourcesIn(ctx context.Context, provider, region string, p NodeProvisioner, refresh bool) (snap resourceSnapshot, cached bool, retry time.Duration, err error) {
	lister, ok := p.(provision.ResourceLister)
	if !ok {
		return snap, false, 0, errResourcesUnsupported
	}
	ttl := time.Duration(rt.tunableInt(ctx, runtimesettings.KeyProvisionResourcesCacheTTL, "APP_PROVISION_RESOURCES_CACHE_TTL_SECONDS", defaultResourcesCacheTTLSecs)) * time.Second
	gap := time.Duration(rt.tunableInt(ctx, runtimesettings.KeyProvisionResourcesRefreshGap, "APP_PROVISION_RESOURCES_REFRESH_MIN_INTERVAL_SECONDS", defaultResourcesRefreshGap)) * time.Second
	slot := provider
	rl, regional := p.(provision.RegionResourceLister)
	if region != "" && regional {
		slot = provider + "@" + region
	}
	return rt.providerCaps.resources.get(slot, ttl, gap, refresh, func() (resourceSnapshot, error) {
		rctx, cancel := context.WithTimeout(ctx, providerReadTimeout)
		defer cancel()
		var list []provision.ProviderResource
		var lerr error
		if region != "" && regional {
			list, lerr = rl.ListServers(rctx, region)
		} else {
			list, lerr = lister.ListResources(rctx)
		}
		return resourceSnapshot{resources: list, at: time.Now().UTC()}, lerr
	})
}

var errResourcesUnsupported = errors.New("api: provider does not support discovery")

// annotateResources fills managed_by_levelrail and enrolled_node on a copy
// of the cached list, so it stays current without re-asking the provider.
func (rt *Router) annotateResources(ctx context.Context, provider string, in []provision.ProviderResource) []provision.ProviderResource {
	out := make([]provision.ProviderResource, len(in))
	copy(out, in)
	provisions, perr := rt.nodeProvisions.ListNodeProvisions(ctx)
	nodes, nerr := rt.nodes.ListNodes(ctx)
	if perr != nil {
		rt.logger.Warn("api: discovery could not read provisions", slog.String("provider", provider))
	}
	for i := range out {
		r := &out[i]
		var nodeID, nodeName string
		for _, t := range r.Tags {
			if t == managedTag || strings.HasPrefix(t, managedProvisionTagPrefix) {
				r.ManagedByLevelrail = true
			}
		}
		if perr == nil {
			for _, pr := range provisions {
				byID := pr.Provider == provider && pr.ProviderServerID != "" && pr.ProviderServerID == r.ID
				byTag := false
				for _, t := range r.Tags {
					byTag = byTag || (pr.ID != "" && t == managedProvisionTagPrefix+pr.ID)
				}
				if !byID && !byTag {
					continue
				}
				r.ManagedByLevelrail = true
				nodeID = pr.NodeID
				if nodeID == "" && nerr == nil {
					if n, ok := findNodeByName(nodes, pr.Name); ok {
						nodeID = n.ID
					}
				}
				break
			}
		}
		if nerr == nil {
			for _, n := range nodes {
				if (nodeID != "" && n.ID == nodeID) || (nodeID == "" && r.PublicIPv4 != "" && addressHost(n.Address) == r.PublicIPv4) {
					nodeID, nodeName = n.ID, n.Name
					break
				}
			}
		}
		if nodeName != "" {
			r.EnrolledNode = &provision.EnrolledNodeRef{ID: nodeID, Name: nodeName}
		}
	}
	return out
}

func regionKnown(regions []provision.Region, id string) bool {
	for _, r := range regions {
		if r.ID == id {
			return true
		}
	}
	return false
}

func addressHost(addr string) string {
	if h, _, err := net.SplitHostPort(addr); err == nil {
		return h
	}
	return addr
}

// writeProviderReadError maps a provider failure to a fixed answer: 424 for
// a missing scope, 501 when unsupported, 502 otherwise. No provider text.
func (rt *Router) writeProviderReadError(w http.ResponseWriter, provider, capability string, err error) {
	if errors.Is(err, errResourcesUnsupported) {
		writeError(w, http.StatusNotImplemented, "this provider does not support listing its machines yet")
		return
	}
	if rt.writeProviderScopeMissing(w, provider, capability, err) {
		return
	}
	rt.logger.Error("api: provider read failed", slog.String("provider", provider), slog.String("reason", providerErrorText(err)))
	writeError(w, http.StatusBadGateway, "could not reach the provider's API")
}

// handleListNodeProviderResources handles GET
// /api/v1/node-providers/{provider}/resources?refresh=1&region=R: the machines in the
// provider account, marked managed or discovered. Read-only.
func (rt *Router) handleListNodeProviderResources(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == nodeProviderCustom {
		writeError(w, http.StatusBadRequest, "a manual data center has no cloud API to list")
		return
	}
	p, ok, status, msg := rt.resolveNodeProvisioner(r.Context(), provider)
	if !ok {
		writeError(w, status, msg)
		return
	}
	region := strings.TrimSpace(r.URL.Query().Get("region"))
	if region != "" {
		if _, regional := p.(provision.RegionResourceLister); regional {
			if known, rerr := p.ListRegions(r.Context()); rerr != nil {
				rt.writeProviderReadError(w, provider, "regions.read", rerr)
				return
			} else if !regionKnown(known, region) {
				writeError(w, http.StatusBadRequest, "unknown region for this provider")
				return
			}
		}
	}
	snap, cached, retry, err := rt.loadProviderResourcesIn(r.Context(), provider, region, p, r.URL.Query().Get("refresh") == "1")
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "the machine list was just refreshed, try again shortly")
		return
	}
	if err != nil {
		rt.writeProviderReadError(w, provider, "instances.read", err)
		return
	}
	list := rt.annotateResources(r.Context(), provider, snap.resources)
	writeJSON(w, http.StatusOK, providerResourcesResponse{
		Provider: provider, AccountID: provider + "-default", GeneratedAt: snap.at,
		Cached: cached, Count: len(list), Resources: list,
	})
}
