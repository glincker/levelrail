package trafficpolicy

import (
	"fmt"
	"net/http"
)

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
		if r.TTLSeconds < 0 || r.TTLSeconds > maxTTL {
			col.add(f+".ttl_seconds", "must be between 0 and %d", maxTTL)
		}
		if r.OverrideUpstream && r.TTLSeconds == 0 {
			col.add(f+".ttl_seconds", "is required when overriding the app's Cache-Control")
		}
		if r.StaleWhileRevalidateSeconds < 0 || r.StaleWhileRevalidateSeconds > maxTTL {
			col.add(f+".stale_while_revalidate_seconds", "must be between 0 and %d", maxTTL)
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
