package ingress

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultVisitorTopN   = 10
	maxVisitorTopN       = 50
	visitorSeriesBuckets = 24
	defaultVisitorSalt   = 24 * time.Hour
)

// VisitorSummaryOptions selects what Summarize aggregates.
type VisitorSummaryOptions struct {
	Domain string
	Since  time.Time
	Until  time.Time
	TopN   int
}

// CountedItem is one ranked label.
type CountedItem struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// VisitorBucket is one sparkline step.
type VisitorBucket struct {
	Start    time.Time `json:"start"`
	Requests int       `json:"requests"`
	Visitors int       `json:"visitors"`
}

// DomainVisitors is the per-domain split of a summary.
type DomainVisitors struct {
	Domain   string `json:"domain"`
	Requests int    `json:"requests"`
	Visitors int    `json:"visitors"`
}

// VisitorSummary is aggregate only: no address, hash or user agent leaves the process.
type VisitorSummary struct {
	Requests   int              `json:"requests"`
	Visitors   int              `json:"visitors"`
	Status     map[string]int   `json:"status"`
	TopPaths   []CountedItem    `json:"top_paths"`
	TopRefs    []CountedItem    `json:"top_referrers"`
	Series     []VisitorBucket  `json:"series"`
	Domains    []DomainVisitors `json:"domains"`
	OldestKept time.Time        `json:"oldest_record,omitempty"`
	SaltPeriod string           `json:"salt_period"`
}

// SetVisitorRotationSource makes the salt period follow a saved setting.
func (l *AccessLog) SetVisitorRotationSource(fn func() time.Duration) {
	if l != nil {
		l.rotationSrc.Store(&fn)
	}
}

func (l *AccessLog) saltPeriod() time.Duration {
	if p := l.rotationSrc.Load(); p != nil {
		if d := (*p)(); d > 0 {
			return d
		}
	}
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("APP_VISITOR_SALT_ROTATION")))
	if err != nil || d <= 0 {
		return defaultVisitorSalt
	}
	return d
}

// visitorSecret is random per process and never persisted, so hashes cannot be replayed after a restart.
func (l *AccessLog) visitorSecret() []byte {
	l.secretOnce.Do(func() {
		l.secret = make([]byte, 32)
		_, _ = rand.Read(l.secret)
	})
	return l.secret
}

func (l *AccessLog) visitorHash(ip string, at time.Time, period time.Duration) string {
	m := hmac.New(sha256.New, l.visitorSecret())
	m.Write([]byte(strconv.FormatInt(at.UnixNano()/int64(period), 10) + "|"))
	m.Write([]byte(ip))
	return hex.EncodeToString(m.Sum(nil)[:8])
}

func visitorTopN(n int) int {
	if n <= 0 {
		if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_VISITOR_TOP_N"))); err == nil && v > 0 {
			n = v
		} else {
			n = defaultVisitorTopN
		}
	}
	if n > maxVisitorTopN {
		n = maxVisitorTopN
	}
	return n
}

func topCounts(m map[string]int, n int) []CountedItem {
	out := make([]CountedItem, 0, len(m))
	for k, v := range m {
		out = append(out, CountedItem{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

// Summarize aggregates retained records for keys into visitor counts, status classes,
// top pages and referrers, and a fixed-step series over [Since, Until].
func (l *AccessLog) Summarize(keys []string, o VisitorSummaryOptions) VisitorSummary {
	if l == nil {
		return VisitorSummary{Status: map[string]int{}}
	}
	period := l.saltPeriod()
	sum := VisitorSummary{Status: map[string]int{"2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0}, SaltPeriod: period.String(),
		TopPaths: []CountedItem{}, TopRefs: []CountedItem{}, Domains: []DomainVisitors{}}
	if o.Until.IsZero() {
		o.Until = time.Now()
	}
	span := o.Until.Sub(o.Since)
	if span <= 0 {
		return sum
	}
	step := span / visitorSeriesBuckets
	if step <= 0 {
		step = time.Second
	}
	series := make([]VisitorBucket, visitorSeriesBuckets)
	bucketSeen := make([]map[string]struct{}, visitorSeriesBuckets)
	for i := range series {
		series[i].Start = o.Since.Add(time.Duration(i) * step)
		bucketSeen[i] = map[string]struct{}{}
	}
	domain := hostOnly(o.Domain)
	seen := map[string]struct{}{}
	paths, refs := map[string]int{}, map[string]int{}
	type dom struct {
		req  int
		seen map[string]struct{}
	}
	doms := map[string]*dom{}
	for _, rec := range l.Since(keys, o.Since) {
		if rec.Time.After(o.Until) {
			continue
		}
		host := hostOnly(rec.Host)
		if domain != "" && host != domain {
			continue
		}
		if sum.OldestKept.IsZero() || rec.Time.Before(sum.OldestKept) {
			sum.OldestKept = rec.Time
		}
		sum.Requests++
		if c := rec.Status / 100; c >= 2 && c <= 5 {
			sum.Status[strconv.Itoa(c)+"xx"]++
		}
		paths[rec.Path]++
		if rec.Referrer != "" {
			refs[rec.Referrer]++
		}
		d := doms[host]
		if d == nil {
			d = &dom{seen: map[string]struct{}{}}
			doms[host] = d
		}
		d.req++
		idx := int(rec.Time.Sub(o.Since) / step)
		if idx >= visitorSeriesBuckets {
			idx = visitorSeriesBuckets - 1
		}
		series[idx].Requests++
		if rec.Client.IsValid() {
			h := l.visitorHash(rec.Client.String(), rec.Time, period)
			seen[h] = struct{}{}
			d.seen[h] = struct{}{}
			bucketSeen[idx][h] = struct{}{}
		}
	}
	for i := range series {
		series[i].Visitors = len(bucketSeen[i])
	}
	sum.Visitors = len(seen)
	sum.Series = series
	n := visitorTopN(o.TopN)
	sum.TopPaths, sum.TopRefs = topCounts(paths, n), topCounts(refs, n)
	for h, d := range doms {
		sum.Domains = append(sum.Domains, DomainVisitors{Domain: h, Requests: d.req, Visitors: len(d.seen)})
	}
	sort.Slice(sum.Domains, func(i, j int) bool {
		if sum.Domains[i].Requests != sum.Domains[j].Requests {
			return sum.Domains[i].Requests > sum.Domains[j].Requests
		}
		return sum.Domains[i].Domain < sum.Domains[j].Domain
	})
	return sum
}
