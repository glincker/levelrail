package ingress

import (
	"container/list"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EnvCacheMaxBytes caps the whole in-process response cache.
const EnvCacheMaxBytes = "APP_INGRESS_CACHE_MAX_BYTES"

const (
	defaultCacheMaxBytes = 64 << 20
	cacheSeriesBuckets   = 30
	cacheEntryOverhead   = 256
)

// Cache purge scopes.
const (
	PurgeURL    = "url"
	PurgePrefix = "prefix"
	PurgeAll    = "all"
	PurgeTag    = "tag"
)

// Cache tunables read at first use, so there are no hardcoded limits.
const (
	// EnvCacheTagHeader names the response header an app uses to tag
	// objects for purge-by-tag.
	EnvCacheTagHeader = "APP_INGRESS_CACHE_TAG_HEADER"
	// EnvCacheRevalidateWindow is how long an expired object with an ETag
	// or Last-Modified is kept so the app can answer 304 instead of resending.
	EnvCacheRevalidateWindow = "APP_INGRESS_CACHE_REVALIDATE_WINDOW"
	// EnvCacheCoalesceWait bounds how long a request waits for an identical
	// in-flight miss before going to the app itself.
	EnvCacheCoalesceWait = "APP_INGRESS_CACHE_COALESCE_WAIT"

	// EnvCacheReasonKeys caps the distinct "why not cached" keys kept per
	// domain; further keys are counted under ReasonOther.
	EnvCacheReasonKeys = "APP_INGRESS_CACHE_REASON_KEYS"
	// EnvCacheTopReasons is how many reasons the API lists per app.
	EnvCacheTopReasons = "APP_INGRESS_CACHE_TOP_REASONS"

	defaultCacheReasonKeys       = 32
	defaultCacheTopReasons       = 5
	defaultCacheTagHeader        = "Cache-Tag"
	defaultCacheRevalidateWindow = time.Hour
	defaultCacheCoalesceWait     = 10 * time.Second
	maxCacheTags                 = 32
	maxCacheTagLen               = 128
)

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && n > 0 {
		return n
	}
	return def
}

// TopReasonCount is how many reasons the API lists per app.
func TopReasonCount() int { return envInt(EnvCacheTopReasons, defaultCacheTopReasons) }

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

// CacheTagHeaderName is the response header apps use to tag cached objects.
func CacheTagHeaderName() string { return cacheTagHeader() }

func cacheTagHeader() string {
	if v := strings.TrimSpace(os.Getenv(EnvCacheTagHeader)); v != "" {
		return http.CanonicalHeaderKey(v)
	}
	return defaultCacheTagHeader
}

type cacheEntry struct {
	key     string
	host    string
	uri     string
	origURI string
	tags    []string
	status  int
	header  http.Header
	body    []byte
	stored  time.Time
	expires time.Time
	stale   time.Duration
	// onError is how long past expiry the entry may stand in for a failing app.
	onError time.Duration
	size    int64
}

func (e *cacheEntry) fresh(now time.Time) bool { return now.Before(e.expires) }

func (e *cacheEntry) servableStale(now time.Time) bool {
	return e.stale > 0 && now.Before(e.expires.Add(e.stale))
}

func (e *cacheEntry) servableOnError(now time.Time) bool {
	return e.onError > 0 && now.Before(e.expires.Add(e.onError))
}

func (e *cacheEntry) hasValidator() bool {
	return e.header.Get("Etag") != "" || e.header.Get("Last-Modified") != ""
}

// retained reports whether an expired entry is still worth keeping: for
// stale serving, as an error fallback, or to revalidate with the app.
func (e *cacheEntry) retained(now time.Time) bool {
	if e.fresh(now) || e.servableStale(now) || e.servableOnError(now) {
		return true
	}
	return e.hasValidator() && now.Before(e.expires.Add(envDuration(EnvCacheRevalidateWindow, defaultCacheRevalidateWindow)))
}

// CacheBucket is one minute of cache activity for the dashboard sparkline.
type CacheBucket struct {
	Minute    int64  `json:"minute"`
	Hits      uint64 `json:"hits"`
	Misses    uint64 `json:"misses"`
	HitBytes  uint64 `json:"hit_bytes"`
	MissBytes uint64 `json:"miss_bytes"`
}

// CacheStats is one domain's cache activity since the process started.
type CacheStats struct {
	Hits     uint64 `json:"hits"`
	Misses   uint64 `json:"misses"`
	Bypasses uint64 `json:"bypasses"`
	Stale    uint64 `json:"stale"`
	// HitBytes were served from the cache; OriginBytes were fetched from the
	// app on cacheable paths. HitBytes is the bandwidth the app was spared.
	HitBytes    uint64        `json:"hit_bytes"`
	OriginBytes uint64        `json:"origin_bytes"`
	Entries     int           `json:"entries"`
	Bytes       int64         `json:"bytes"`
	Series      []CacheBucket `json:"series"`
	// Reasons counts responses and requests left out of the cache, most
	// common first.
	Reasons []CacheReason `json:"reasons"`
}

type domainCacheStats struct {
	hits, misses, bypasses, stale uint64
	hitBytes, originBytes         uint64
	entries                       int
	bytes                         int64
	reasons                       map[string]uint64
	series                        [cacheSeriesBuckets]CacheBucket
}

func (d *domainCacheStats) bucket(now time.Time) *CacheBucket {
	minute := now.Unix() / 60
	b := &d.series[minute%cacheSeriesBuckets]
	if b.Minute != minute {
		*b = CacheBucket{Minute: minute}
	}
	return b
}

// ResponseCache is a bounded LRU of whole responses shared by every cache
// handler in the process. Memory stays under MaxBytes: inserting evicts
// least recently used entries until the new one fits.
type ResponseCache struct {
	mu         sync.Mutex
	maxBytes   int64
	used       int64
	ll         *list.List
	items      map[string]*list.Element
	stats      map[string]*domainCacheStats
	refreshing map[string]chan struct{}
	now        func() time.Time
}

// NewResponseCache returns an empty cache capped at maxBytes.
func NewResponseCache(maxBytes int64) *ResponseCache {
	if maxBytes <= 0 {
		maxBytes = defaultCacheMaxBytes
	}
	return &ResponseCache{
		maxBytes:   maxBytes,
		ll:         list.New(),
		items:      map[string]*list.Element{},
		stats:      map[string]*domainCacheStats{},
		refreshing: map[string]chan struct{}{},
		now:        time.Now,
	}
}

var (
	responseCacheOnce sync.Once
	responseCache     *ResponseCache
)

// DefaultResponseCache is the process-wide cache the cache handler uses,
// sized from APP_INGRESS_CACHE_MAX_BYTES.
func DefaultResponseCache() *ResponseCache {
	responseCacheOnce.Do(func() {
		limit, _ := strconv.ParseInt(os.Getenv(EnvCacheMaxBytes), 10, 64)
		responseCache = NewResponseCache(limit)
	})
	return responseCache
}

// MaxBytes is the global byte cap.
func (c *ResponseCache) MaxBytes() int64 { return c.maxBytes }

func (c *ResponseCache) domain(host string) *domainCacheStats {
	d := c.stats[host]
	if d == nil {
		d = &domainCacheStats{}
		c.stats[host] = d
	}
	return d
}

// lookup returns the entry for key and whether it is fresh. A stale but
// servable entry is returned with fresh false and stale true, and only
// while another request is refreshing it.
func (c *ResponseCache) lookup(key string) (e *cacheEntry, fresh, stale bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false, false
	}
	e = el.Value.(*cacheEntry)
	now := c.now()
	if e.fresh(now) {
		c.ll.MoveToFront(el)
		return e, true, false
	}
	if _, busy := c.refreshing[key]; busy && e.servableStale(now) {
		return e, false, true
	}
	if !e.retained(now) {
		c.removeLocked(el)
	}
	return nil, false, false
}

// peek returns the entry for key without touching recency or removing it,
// whether or not it is fresh.
func (c *ResponseCache) peek(key string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		return el.Value.(*cacheEntry)
	}
	return nil
}

// beginRefresh marks key as being fetched; false if another request is.
func (c *ResponseCache) beginRefresh(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, busy := c.refreshing[key]; busy {
		return false
	}
	c.refreshing[key] = make(chan struct{})
	return true
}

// inflight returns a channel closed when the fetch for key finishes, or nil
// when nothing is being fetched.
func (c *ResponseCache) inflight(key string) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshing[key]
}

func (c *ResponseCache) endRefresh(key string) {
	c.mu.Lock()
	if ch, ok := c.refreshing[key]; ok {
		close(ch)
		delete(c.refreshing, key)
	}
	c.mu.Unlock()
}

func (c *ResponseCache) put(e *cacheEntry) {
	e.size = int64(len(e.body)) + headerSize(e.header) + int64(len(e.key)) + cacheEntryOverhead
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.size > c.maxBytes {
		return
	}
	if el, ok := c.items[e.key]; ok {
		c.removeLocked(el)
	}
	for c.used+e.size > c.maxBytes {
		back := c.ll.Back()
		if back == nil {
			break
		}
		c.removeLocked(back)
	}
	c.items[e.key] = c.ll.PushFront(e)
	c.used += e.size
	d := c.domain(e.host)
	d.entries++
	d.bytes += e.size
}

func (c *ResponseCache) removeLocked(el *list.Element) {
	e := el.Value.(*cacheEntry)
	c.ll.Remove(el)
	delete(c.items, e.key)
	c.used -= e.size
	d := c.domain(e.host)
	d.entries--
	d.bytes -= e.size
}

func headerSize(h http.Header) int64 {
	var n int64
	for k, vs := range h {
		for _, v := range vs {
			n += int64(len(k) + len(v) + 4)
		}
	}
	return n
}

func (c *ResponseCache) record(host string, outcome string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.domain(host)
	b := d.bucket(c.now())
	switch outcome {
	case cacheHit:
		d.hits++
		b.Hits++
	case cacheStale:
		d.stale++
		b.Hits++
	case cacheMiss:
		d.misses++
		b.Misses++
	default:
		d.bypasses++
	}
}

// recordReasons counts each key (a Reason* code, or code and detail joined
// by reasonSep) once, folding keys past the cap into ReasonOther.
func (c *ResponseCache) recordReasons(host string, keys ...string) {
	if len(keys) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.domain(host)
	if d.reasons == nil {
		d.reasons = map[string]uint64{}
	}
	limit := envInt(EnvCacheReasonKeys, defaultCacheReasonKeys)
	for _, k := range keys {
		if _, seen := d.reasons[k]; !seen && len(d.reasons) >= limit {
			k = ReasonOther
		}
		d.reasons[k]++
	}
}

// recordBytes counts body bytes served from the cache (fromCache) or
// fetched from the app on a cacheable path.
func (c *ResponseCache) recordBytes(host string, fromCache bool, n int64) {
	if n <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.domain(host)
	b := d.bucket(c.now())
	if fromCache {
		d.hitBytes += uint64(n)
		b.HitBytes += uint64(n)
		return
	}
	d.originBytes += uint64(n)
	b.MissBytes += uint64(n)
}

// Purge removes host's entries: one URL (path and query, every Vary
// variant), every path under a prefix, or all of them. Returns the count.
func (c *ResponseCache) Purge(host, scope, value string) int {
	host = strings.ToLower(host)
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for el := c.ll.Front(); el != nil; {
		next := el.Next()
		e := el.Value.(*cacheEntry)
		if e.host == host && e.purgedBy(scope, value) {
			c.removeLocked(el)
			n++
		}
		el = next
	}
	return n
}

func purgeMatches(scope, value, uri string) bool {
	switch scope {
	case PurgeAll:
		return true
	case PurgePrefix:
		path, _, _ := strings.Cut(uri, "?")
		return strings.HasPrefix(path, value)
	default:
		return uri == value
	}
}

func (e *cacheEntry) purgedBy(scope, value string) bool {
	switch scope {
	case PurgeTag:
		return slices.Contains(e.tags, value)
	case PurgeURL:
		return e.uri == value || e.origURI == value
	}
	return purgeMatches(scope, value, e.uri)
}

// Stats returns host's counters and the last 30 minutes of activity,
// oldest first.
func (c *ResponseCache) Stats(host string) CacheStats {
	host = strings.ToLower(host)
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.stats[host]
	if d == nil {
		d = &domainCacheStats{}
	}
	out := CacheStats{Hits: d.hits, Misses: d.misses, Bypasses: d.bypasses, Stale: d.stale, HitBytes: d.hitBytes, OriginBytes: d.originBytes, Entries: d.entries, Bytes: d.bytes}
	out.Series = c.seriesLocked(d)
	out.Reasons = sortedReasons(d.reasons)
	return out
}

func (c *ResponseCache) seriesLocked(d *domainCacheStats) []CacheBucket {
	var out []CacheBucket
	nowMinute := c.now().Unix() / 60
	for i := int64(cacheSeriesBuckets - 1); i >= 0; i-- {
		m := nowMinute - i
		b := d.series[m%cacheSeriesBuckets]
		if b.Minute != m {
			b = CacheBucket{Minute: m}
		}
		out = append(out, b)
	}
	return out
}

// Hosts lists every host that has cache activity or entries.
func (c *ResponseCache) Hosts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.stats))
	for h := range c.stats {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// UsedBytes is the current total size of every entry.
func (c *ResponseCache) UsedBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}
