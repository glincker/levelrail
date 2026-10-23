package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const (
	maxPurgeValueLen = 2048
	maxPurgeTagLen   = 128
	purgeHistoryPage = 50
)

// CachePurgeStore keeps the per-app purge history shown on the Caches page.
type CachePurgeStore interface {
	AddCachePurge(ctx context.Context, p store.CachePurge) error
	ListCachePurges(ctx context.Context, app string, limit int) ([]store.CachePurge, error)
}

type cacheNumbers struct {
	Hits        uint64  `json:"hits"`
	Misses      uint64  `json:"misses"`
	Stale       uint64  `json:"stale"`
	Bypasses    uint64  `json:"bypasses"`
	HitRate     float64 `json:"hit_rate"`
	HitBytes    uint64  `json:"hit_bytes"`
	OriginBytes uint64  `json:"origin_bytes"`
	Entries     int     `json:"entries"`
	Bytes       int64   `json:"bytes"`
}

func numbersOf(s ingress.CacheStats) cacheNumbers {
	return cacheNumbers{
		Hits: s.Hits, Misses: s.Misses, Stale: s.Stale, Bypasses: s.Bypasses, HitRate: s.HitRate(),
		HitBytes: s.HitBytes, OriginBytes: s.OriginBytes, Entries: s.Entries, Bytes: s.Bytes,
	}
}

// cacheReason is one "why not cached" entry for an app. Domains lists the
// hosts it was seen on so a fix can target them.
type cacheReason struct {
	Code    string   `json:"code"`
	Detail  string   `json:"detail,omitempty"`
	Count   uint64   `json:"count"`
	Domains []string `json:"domains"`
}

// appReasons merges the domains' reasons, keeps the top few and notes which
// domains each applies to.
func appReasons(domains []string, stats []ingress.CacheStats) []cacheReason {
	merged := ingress.MergeCacheStats(stats...).Reasons
	out := make([]cacheReason, 0, len(merged))
	for _, r := range merged[:min(len(merged), ingress.TopReasonCount())] {
		cr := cacheReason{Code: r.Code, Detail: r.Detail, Count: r.Count, Domains: []string{}}
		for i, st := range stats {
			if slices.ContainsFunc(st.Reasons, func(x ingress.CacheReason) bool { return x.Code == r.Code && x.Detail == r.Detail }) {
				cr.Domains = append(cr.Domains, domains[i])
			}
		}
		out = append(out, cr)
	}
	return out
}

// suggestedPreset names a preset when the reasons show an upstream the
// preset was made for.
func suggestedPreset(reasons []cacheReason) string {
	var names []string
	for _, r := range reasons {
		if r.Code == ingress.ReasonVaryHeader {
			names = append(names, r.Detail)
		}
	}
	if trafficpolicy.MatchesNextJSVary(names) {
		return trafficpolicy.PresetNextJS
	}
	return ""
}

type cacheLimits struct {
	MaxBytes       int64  `json:"max_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	MaxObjectBytes int64  `json:"max_object_bytes"`
	TagHeader      string `json:"tag_header"`
	Location       string `json:"location"`
}

// cacheLocationIngress states where entries live: one in-process cache in
// the ingress that serves every app, whichever node runs the app.
const cacheLocationIngress = "control-plane-ingress"

func (rt *Router) cacheLimits() cacheLimits {
	c := ingress.DefaultResponseCache()
	return cacheLimits{
		MaxBytes: c.MaxBytes(), UsedBytes: c.UsedBytes(), MaxObjectBytes: rt.domainPolicy.limits().CacheMaxObject,
		TagHeader: ingress.CacheTagHeaderName(), Location: cacheLocationIngress,
	}
}

type appCacheDomain struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
	Rules   int    `json:"rules"`
	cacheNumbers
}

type appCacheResponse struct {
	App     string                    `json:"app"`
	Limits  cacheLimits               `json:"limits"`
	Totals  cacheNumbers              `json:"totals"`
	Series  []ingress.CacheBucket     `json:"series"`
	Domains []appCacheDomain          `json:"domains"`
	Preset  []trafficpolicy.CacheRule `json:"static_preset"`
	// NextJSPreset is the rule set behind suggest_preset "nextjs".
	NextJSPreset  []trafficpolicy.CacheRule `json:"nextjs_preset"`
	Reasons       []cacheReason             `json:"reasons"`
	SuggestPreset string                    `json:"suggest_preset,omitempty"`
}

func (rt *Router) cachePolicyFor(ctx context.Context, domain string) (trafficpolicy.Cache, error) {
	var c trafficpolicy.Cache
	row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(ctx, domain, trafficpolicy.KindCache)
	if err != nil || !found {
		return c, err
	}
	if err := json.Unmarshal(row.Spec, &c); err != nil {
		return trafficpolicy.Cache{}, err
	}
	return c, nil
}

func (rt *Router) handleGetAppCache(w http.ResponseWriter, r *http.Request) {
	app, domains, ok := rt.appDomains(w, r)
	if !ok {
		return
	}
	resp := appCacheResponse{App: app, Limits: rt.cacheLimits(), Domains: []appCacheDomain{}, Preset: trafficpolicy.StaticAssetsPreset(), NextJSPreset: trafficpolicy.NextJSPreset()}
	var all []ingress.CacheStats
	for _, d := range domains {
		pol, err := rt.cachePolicyFor(r.Context(), d)
		if err != nil {
			rt.internalError(w, "api: app cache: load policy failed", err, slog.String("domain", d))
			return
		}
		st := ingress.DefaultResponseCache().Stats(d)
		all = append(all, st)
		resp.Domains = append(resp.Domains, appCacheDomain{Domain: d, Enabled: pol.Enabled, Rules: len(pol.Rules), cacheNumbers: numbersOf(st)})
	}
	merged := ingress.MergeCacheStats(all...)
	resp.Totals, resp.Series = numbersOf(merged), merged.Series
	if resp.Series == nil {
		resp.Series = []ingress.CacheBucket{}
	}
	resp.Reasons = appReasons(domains, all)
	resp.SuggestPreset = suggestedPreset(resp.Reasons)
	writeJSON(w, http.StatusOK, resp)
}

// validatePurge returns a user-facing message when scope or value is bad.
func validatePurge(scope, value string) string {
	switch scope {
	case ingress.PurgeAll:
		return ""
	case ingress.PurgeURL, ingress.PurgePrefix:
		if !strings.HasPrefix(value, "/") || len(value) > maxPurgeValueLen {
			return "value must be a path starting with / (a URL path and query for scope url)"
		}
	case ingress.PurgeTag:
		if value == "" || len(value) > maxPurgeTagLen || strings.ContainsAny(value, ", \t\r\n") {
			return "value must be one cache tag without spaces or commas"
		}
	default:
		return "scope must be url, prefix, tag or all"
	}
	return ""
}

type purgeDomainResult struct {
	Domain  string `json:"domain"`
	Removed int    `json:"removed"`
}

type appPurgeResponse struct {
	App     string              `json:"app"`
	Scope   string              `json:"scope"`
	Value   string              `json:"value,omitempty"`
	Removed int                 `json:"removed"`
	Domains []purgeDomainResult `json:"domains"`
}

// purgeDomains purges each domain and records one history row per domain.
func (rt *Router) purgeDomains(r *http.Request, app string, domains []string, scope, value string) appPurgeResponse {
	_, _, actor, ok := rt.currentActor(r)
	if !ok || actor == "" {
		actor = "unknown"
	}
	out := appPurgeResponse{App: app, Scope: scope, Value: value, Domains: []purgeDomainResult{}}
	for _, d := range domains {
		n := ingress.DefaultResponseCache().Purge(d, scope, value)
		out.Removed += n
		out.Domains = append(out.Domains, purgeDomainResult{Domain: d, Removed: n})
		if err := rt.cachePurges.AddCachePurge(r.Context(), store.CachePurge{App: app, Domain: d, Scope: scope, Value: value, Removed: n, Actor: actor}); err != nil {
			rt.logger.Warn("api: cache purge history not recorded", slog.String("error", err.Error()), slog.String("app", app), slog.String("domain", d))
		}
	}
	return out
}

func (rt *Router) handlePurgeAppCache(w http.ResponseWriter, r *http.Request) {
	app, domains, ok := rt.appDomains(w, r)
	if !ok {
		return
	}
	var req purgeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validatePurge(req.Scope, req.Value); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.Domain != "" {
		d := strings.ToLower(strings.TrimSpace(req.Domain))
		if !containsFold(domains, d) {
			writeError(w, http.StatusNotFound, "domain is not one of the app's domains")
			return
		}
		domains = []string{d}
	}
	writeJSON(w, http.StatusOK, rt.purgeDomains(r, app, domains, req.Scope, req.Value))
}

type purgeHistoryRow struct {
	Domain  string `json:"domain"`
	Scope   string `json:"scope"`
	Value   string `json:"value,omitempty"`
	Removed int    `json:"removed"`
	Actor   string `json:"actor"`
	At      string `json:"at"`
}

func (rt *Router) handleListCachePurges(w http.ResponseWriter, r *http.Request) {
	app := r.PathValue("name")
	if _, found, err := rt.lookupAppResource(r.Context(), app); err != nil {
		rt.internalError(w, "api: cache purges: load app", err)
		return
	} else if !found {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	rows, err := rt.cachePurges.ListCachePurges(r.Context(), app, purgeHistoryPage)
	if err != nil {
		rt.internalError(w, "api: cache purges: list", err)
		return
	}
	out := make([]purgeHistoryRow, 0, len(rows))
	for _, p := range rows {
		out = append(out, purgeHistoryRow{Domain: p.Domain, Scope: p.Scope, Value: p.Value, Removed: p.Removed, Actor: p.Actor, At: p.At.UTC().Format("2006-01-02T15:04:05Z")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": app, "purges": out})
}

type presetRequest struct {
	Domain string `json:"domain,omitempty"`
}

type presetDomainResult struct {
	Domain  string `json:"domain"`
	Applied bool   `json:"applied"`
	Rules   int    `json:"rules"`
}

// handleApplyCachePreset adds the static assets rule to each (or one)
// domain's cache policy and turns caching on; running it twice is a no-op.
func (rt *Router) handleApplyCachePreset(w http.ResponseWriter, r *http.Request) {
	app, domains, ok := rt.appDomains(w, r)
	if !ok {
		return
	}
	var req presetRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if req.Domain != "" {
		d := strings.ToLower(strings.TrimSpace(req.Domain))
		if !containsFold(domains, d) {
			writeError(w, http.StatusNotFound, "domain is not one of the app's domains")
			return
		}
		domains = []string{d}
	}
	results := []presetDomainResult{}
	for _, d := range domains {
		res, status, err := rt.applyPresetToDomain(r.Context(), d)
		if err != nil {
			if status == http.StatusBadRequest {
				writePolicyError(w, err)
				return
			}
			rt.internalError(w, "api: cache preset failed", err, slog.String("domain", d), slog.String("app", app))
			return
		}
		results = append(results, res)
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, map[string]any{"app": app, "domains": results})
}

func (rt *Router) applyPresetToDomain(ctx context.Context, domain string) (presetDomainResult, int, error) {
	pol, err := rt.cachePolicyFor(ctx, domain)
	if err != nil {
		return presetDomainResult{}, http.StatusInternalServerError, err
	}
	applied := false
	for _, add := range trafficpolicy.StaticAssetsPreset() {
		have := false
		for _, r := range pol.Rules {
			have = have || (r.Match.Kind == add.Match.Kind && r.Match.Path == add.Match.Path)
		}
		if !have {
			pol.Rules, applied = append(pol.Rules, add), true
		}
	}
	if !pol.Enabled {
		pol.Enabled, applied = true, true
	}
	if !applied {
		return presetDomainResult{Domain: domain, Rules: len(pol.Rules)}, http.StatusOK, nil
	}
	pctx, err := rt.policyContext(ctx, domain)
	if err != nil {
		return presetDomainResult{}, http.StatusInternalServerError, err
	}
	if err := (trafficpolicy.Policy{Cache: &pol}).Validate(rt.domainPolicy.limits(), pctx); err != nil {
		return presetDomainResult{}, http.StatusBadRequest, err
	}
	spec, err := json.Marshal(pol)
	if err != nil {
		return presetDomainResult{}, http.StatusInternalServerError, err
	}
	if err := rt.domainPolicy.store.SetDomainTrafficPolicy(ctx, domain, trafficpolicy.KindCache, spec); err != nil {
		return presetDomainResult{}, http.StatusInternalServerError, err
	}
	return presetDomainResult{Domain: domain, Applied: true, Rules: len(pol.Rules)}, http.StatusOK, nil
}

type systemCacheApp struct {
	App            string `json:"app"`
	Domains        int    `json:"domains"`
	EnabledDomains int    `json:"enabled_domains"`
	Requests       uint64 `json:"requests"`
	cacheNumbers
	Reasons       []cacheReason `json:"reasons"`
	SuggestPreset string        `json:"suggest_preset,omitempty"`
}

type systemCacheResponse struct {
	Limits cacheLimits           `json:"limits"`
	Totals cacheNumbers          `json:"totals"`
	Series []ingress.CacheBucket `json:"series"`
	Apps   []systemCacheApp      `json:"apps"`
}

func (rt *Router) handleSystemCache(w http.ResponseWriter, r *http.Request) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: system cache: list apps", err)
		return
	}
	resp := systemCacheResponse{Limits: rt.cacheLimits(), Apps: []systemCacheApp{}}
	var all []ingress.CacheStats
	for _, svc := range svcs {
		row := systemCacheApp{App: svc.Name, Domains: len(svc.Domains)}
		var per []ingress.CacheStats
		for _, d := range svc.Domains {
			pol, perr := rt.cachePolicyFor(r.Context(), d)
			if perr != nil {
				rt.internalError(w, "api: system cache: load policy failed", perr, slog.String("domain", d))
				return
			}
			if pol.Enabled {
				row.EnabledDomains++
			}
			per = append(per, ingress.DefaultResponseCache().Stats(d))
		}
		merged := ingress.MergeCacheStats(per...)
		all = append(all, merged)
		row.cacheNumbers = numbersOf(merged)
		row.Requests = merged.Hits + merged.Stale + merged.Misses + merged.Bypasses
		row.Reasons = appReasons(svc.Domains, per)
		row.SuggestPreset = suggestedPreset(row.Reasons)
		resp.Apps = append(resp.Apps, row)
	}
	sort.Slice(resp.Apps, func(i, j int) bool {
		if resp.Apps[i].Requests != resp.Apps[j].Requests {
			return resp.Apps[i].Requests > resp.Apps[j].Requests
		}
		return resp.Apps[i].App < resp.Apps[j].App
	})
	merged := ingress.MergeCacheStats(all...)
	resp.Totals, resp.Series = numbersOf(merged), merged.Series
	if resp.Series == nil {
		resp.Series = []ingress.CacheBucket{}
	}
	writeJSON(w, http.StatusOK, resp)
}
