// Package pages holds the privacy-friendly analytics view for static sites.
package pages

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Count is a label with how many requests it covers.
type Count struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// StatusCount is one exact response code with its request count.
type StatusCount struct {
	Code  int `json:"code"`
	Count int `json:"count"`
}

// Analytics is the body of GET /api/v1/static-sites/{name}/analytics. It holds
// no client address, cookie, country or query string.
type Analytics struct {
	Site          string        `json:"site"`
	Range         string        `json:"range"`
	Since         time.Time     `json:"since"`
	GeneratedAt   time.Time     `json:"generated_at"`
	RetentionDays int           `json:"retention_days"`
	Requests      int           `json:"requests"`
	TopPages      []Count       `json:"top_pages"`
	TopReferrers  []Count       `json:"top_referrers"`
	Status        []StatusCount `json:"status"`
	Partial       bool          `json:"partial"`
}

// Ranges are the windows the endpoint serves.
var Ranges = map[string]time.Duration{"24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}

// RangeFits reports whether a range is inside the retention in days.
func RangeFits(span time.Duration, retentionDays int) bool {
	return span <= time.Duration(retentionDays)*24*time.Hour
}

// RefHost reduces a referrer to a lowercase host, or "" when none is usable.
// Paths, queries and credentials are dropped so nothing identifying is kept.
func RefHost(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		if _, err := strconv.Atoi(s[i+1:]); err == nil {
			s = s[:i]
		}
	}
	return strings.TrimPrefix(strings.ToLower(s), "www.")
}

// MergeReferrers folds referrer labels to hosts, merges duplicates and keeps the top n.
func MergeReferrers(in []Count, n int) []Count {
	m := map[string]int{}
	for _, c := range in {
		if h := RefHost(c.Label); h != "" {
			m[h] += c.Count
		}
	}
	out := make([]Count, 0, len(m))
	for h, c := range m {
		out = append(out, Count{Label: h, Count: c})
	}
	sortCounts(out)
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// StatusBreakdown counts exact response codes, most frequent first.
func StatusBreakdown(codes []int) []StatusCount {
	m := map[int]int{}
	for _, c := range codes {
		if c >= 100 && c <= 599 {
			m[c]++
		}
	}
	out := make([]StatusCount, 0, len(m))
	for c, n := range m {
		out = append(out, StatusCount{Code: c, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func sortCounts(c []Count) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Count != c[j].Count {
			return c[i].Count > c[j].Count
		}
		return c[i].Label < c[j].Label
	})
}
