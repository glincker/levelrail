package trafficpolicy

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

const maxCacheKeyNames = 16

// CacheRule selects responses to keep in the ingress cache.
type CacheRule struct {
	Match Match `json:"match"`
	// TTLSeconds applies when the app sends no max-age or s-maxage, or
	// always when OverrideUpstream is set.
	TTLSeconds       int  `json:"ttl_seconds"`
	OverrideUpstream bool `json:"override_upstream,omitempty"`
	// StaleWhileRevalidateSeconds serves an expired entry while one
	// background request refreshes it.
	StaleWhileRevalidateSeconds int `json:"stale_while_revalidate_seconds,omitempty"`
	// StatusCodes cached; empty means 200 only.
	StatusCodes []int `json:"status_codes,omitempty"`
	// Vary lists request headers that split the cache key.
	Vary []string `json:"vary,omitempty"`
	// CacheWithCookies keeps caching when the request carries a Cookie.
	// Off by default: a cookie usually means a personalised page.
	CacheWithCookies bool `json:"cache_with_cookies,omitempty"`
	// Never marks paths that must not be cached: the first matching rule
	// wins, so list these before broader cache rules.
	Never bool `json:"never,omitempty"`
	// StaleIfErrorSeconds serves an expired entry, marked STALE, when the
	// app answers 5xx or cannot be reached.
	StaleIfErrorSeconds int `json:"stale_if_error_seconds,omitempty"`
	// IgnoreQueryParams are dropped from the cache key (tracking params).
	IgnoreQueryParams []string `json:"ignore_query_params,omitempty"`
	// KeyCookies adds these cookies' values to the cache key; requires
	// CacheWithCookies.
	KeyCookies []string `json:"key_cookies,omitempty"`
}

// Cache is a domain's cache policy.
type Cache struct {
	Enabled        bool        `json:"enabled"`
	Rules          []CacheRule `json:"rules"`
	MaxObjectBytes int64       `json:"max_object_bytes,omitempty"`
}

var cacheMethods = map[string]bool{http.MethodGet: true, http.MethodHead: true}

var cacheableStatuses = map[int]bool{200: true, 203: true, 204: true, 300: true, 301: true, 308: true, 404: true, 405: true, 410: true, 414: true, 501: true}

// EffectiveStatusCodes returns the statuses the rule stores.
func (r CacheRule) EffectiveStatusCodes() []int {
	if len(r.StatusCodes) == 0 {
		return []int{http.StatusOK}
	}
	return r.StatusCodes
}

// Validate checks every rule against the instance caps.
func (c *Cache) Validate(l Limits) error {
	col := &collector{kind: KindCache}
	if len(c.Rules) > l.MaxCacheRules {
		col.add("rules", "at most %d cache rules per domain", l.MaxCacheRules)
	}
	if c.Enabled && len(c.Rules) == 0 {
		col.add("rules", "add at least one rule, or turn caching off")
	}
	if c.MaxObjectBytes < 0 || c.MaxObjectBytes > l.CacheMaxObject {
		col.add("max_object_bytes", "must be between 0 (default) and %d", l.CacheMaxObject)
	}
	maxTTL := int(l.CacheMaxTTL.Seconds())
	for i, r := range c.Rules {
		f := fmt.Sprintf("rules[%d]", i)
		validateMatch(col, f+".match", r.Match, l, cacheMethods)
		if r.Never {
			continue
		}
		if r.TTLSeconds < 0 || r.TTLSeconds > maxTTL {
			col.add(f+".ttl_seconds", "must be between 0 and %d", maxTTL)
		}
		if r.OverrideUpstream && r.TTLSeconds == 0 {
			col.add(f+".ttl_seconds", "is required when overriding the app's Cache-Control")
		}
		if r.StaleWhileRevalidateSeconds < 0 || r.StaleWhileRevalidateSeconds > maxTTL {
			col.add(f+".stale_while_revalidate_seconds", "must be between 0 and %d", maxTTL)
		}
		if r.StaleIfErrorSeconds < 0 || r.StaleIfErrorSeconds > maxTTL {
			col.add(f+".stale_if_error_seconds", "must be between 0 and %d", maxTTL)
		}
		if len(r.IgnoreQueryParams) > maxCacheKeyNames {
			col.add(f+".ignore_query_params", "at most %d names", maxCacheKeyNames)
		}
		for j, q := range r.IgnoreQueryParams {
			if q == "" || len(q) > 64 || strings.ContainsAny(q, "&=#") {
				col.add(fmt.Sprintf("%s.ignore_query_params[%d]", f, j), "must be a bare parameter name")
			}
		}
		if len(r.KeyCookies) > maxCacheKeyNames {
			col.add(f+".key_cookies", "at most %d names", maxCacheKeyNames)
		}
		if len(r.KeyCookies) > 0 && !r.CacheWithCookies {
			col.add(f+".key_cookies", "requires cache_with_cookies")
		}
		for j, c := range r.KeyCookies {
			if !IsToken(c) {
				col.add(fmt.Sprintf("%s.key_cookies[%d]", f, j), "must be a cookie name")
			}
		}
		for j, s := range r.StatusCodes {
			if !cacheableStatuses[s] {
				col.add(fmt.Sprintf("%s.status_codes[%d]", f, j), "%d is not a cacheable status", s)
			}
		}
		if len(r.Vary) > 8 {
			col.add(f+".vary", "at most 8 headers")
		}
		for j, v := range r.Vary {
			canon := http.CanonicalHeaderKey(v)
			if !IsToken(v) || canon == "Cookie" || canon == "Authorization" {
				col.add(fmt.Sprintf("%s.vary[%d]", f, j), "must be a header name other than Cookie or Authorization")
			}
		}
	}
	return col.err()
}

// StaticAssetsPreset caches common static file types for an hour when the
// app sends no Cache-Control of its own; an app's max-age or s-maxage wins,
// so fingerprinted assets sent as immutable keep their long lifetime.
func StaticAssetsPreset() []CacheRule {
	return []CacheRule{{
		Match:                       Match{Kind: MatchRegex, Path: `(?i)\.(css|js|mjs|map|png|jpe?g|gif|webp|avif|svg|ico|woff2?|ttf|otf|eot|mp4|webm|mp3|wasm)$`, Methods: []string{http.MethodGet, http.MethodHead}},
		TTLSeconds:                  3600,
		StaleWhileRevalidateSeconds: 60,
		StaleIfErrorSeconds:         86400,
	}}
}

// Preset names accepted by Presets.
const (
	PresetStaticAssets = "static"
	PresetNextJS       = "nextjs"
)

// nextJSVary are the request headers Next.js varies its responses on.
var nextJSVary = []string{"rsc", "next-router-state-tree", "next-router-prefetch", "next-router-segment-prefetch"}

// NextJSVary returns the request headers a Next.js app varies on.
func NextJSVary() []string { return slices.Clone(nextJSVary) }

// MatchesNextJSVary reports whether names include the Next.js router
// headers, which is how a Next.js upstream shows up in a Vary header.
func MatchesNextJSVary(names []string) bool {
	for _, n := range names {
		if strings.EqualFold(n, "rsc") || strings.EqualFold(n, "next-router-state-tree") {
			return true
		}
	}
	return false
}

// NextJSPreset keys cached pages by the router headers so HTML and RSC
// payloads never mix, and keeps fingerprinted build files for a day. The
// app's own Cache-Control still wins, so dynamic pages stay uncached.
func NextJSPreset() []CacheRule {
	get := []string{http.MethodGet, http.MethodHead}
	return []CacheRule{
		{
			Match:                       Match{Kind: MatchPrefix, Path: "/_next/static", Methods: get},
			TTLSeconds:                  86400,
			StaleWhileRevalidateSeconds: 60,
			StaleIfErrorSeconds:         86400,
		},
		{
			Match:                       Match{Kind: MatchPrefix, Path: "/", Methods: get},
			TTLSeconds:                  60,
			StaleWhileRevalidateSeconds: 30,
			StaleIfErrorSeconds:         600,
			Vary:                        NextJSVary(),
		},
	}
}

// Presets lists the named rule sets the API and CLI offer.
func Presets() map[string][]CacheRule {
	return map[string][]CacheRule{
		PresetStaticAssets: StaticAssetsPreset(),
		PresetNextJS:       NextJSPreset(),
	}
}
