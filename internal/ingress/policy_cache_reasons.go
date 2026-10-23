package ingress

import (
	"sort"
	"strings"
)

// Reason codes for why a request or response stays out of the cache.
const (
	ReasonVaryHeader      = "vary_header"
	ReasonSetCookie       = "set_cookie"
	ReasonPrivate         = "private"
	ReasonNoStore         = "no_store"
	ReasonNoCache         = "no_cache"
	ReasonStatusNotCached = "status_not_cached"
	ReasonTooLarge        = "too_large"
	ReasonRange           = "range"
	ReasonAuthorization   = "authorization"
	ReasonCookieRequest   = "cookie_request"
	ReasonUpgrade         = "upgrade"
	// ReasonOther collects keys past the per-domain cap.
	ReasonOther = "other"

	// reasonSep joins a code to its detail, such as the Vary header name.
	reasonSep = ":"
)

// CacheReason is one reason with how often it applied. Detail carries the
// Vary header name for ReasonVaryHeader.
type CacheReason struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
	Count  uint64 `json:"count"`
}

func splitReason(key string) (code, detail string) {
	code, detail, _ = strings.Cut(key, reasonSep)
	return code, detail
}

func sortedReasons(m map[string]uint64) []CacheReason {
	out := make([]CacheReason, 0, len(m))
	for k, n := range m {
		code, detail := splitReason(k)
		out = append(out, CacheReason{Code: code, Detail: detail, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}

// mergeReasons sums reasons from several domains, most common first.
func mergeReasons(lists ...[]CacheReason) []CacheReason {
	sum := map[string]uint64{}
	for _, l := range lists {
		for _, r := range l {
			k := r.Code
			if r.Detail != "" {
				k += reasonSep + r.Detail
			}
			sum[k] += r.Count
		}
	}
	return sortedReasons(sum)
}
