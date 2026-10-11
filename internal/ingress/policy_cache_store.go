package ingress

import (
	"container/list"
	"net/http"
	"os"
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
)

type cacheEntry struct {
	key     string
	host    string
	uri     string
	status  int
	header  http.Header
	body    []byte
	stored  time.Time
	expires time.Time
	stale   time.Duration
	size    int64
}

func (e *cacheEntry) fresh(now time.Time) bool { return now.Before(e.expires) }

func (e *cacheEntry) servableStale(now time.Time) bool {
	return e.stale > 0 && now.Before(e.expires.Add(e.stale))
}

// CacheBucket is one minute of cache activity for the dashboard sparkline.
type CacheBucket struct {
	Minute int64  `json:"minute"`
	Hits   uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
}

// CacheStats is one domain's cache activity since the process started.
type CacheStats struct {
	Hits     uint64        `json:"hits"`
	Misses   uint64        `json:"misses"`
	Bypasses uint64        `json:"bypasses"`
	Stale    uint64        `json:"stale"`
	Entries  int           `json:"entries"`
	Bytes    int64         `json:"bytes"`
	Series   []CacheBucket `json:"series"`
}

type domainCacheStats struct {
	hits, misses, bypasses, stale uint64
	entries                       int
	bytes                         int64
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
	refreshing map[string]bool
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
		refreshing: map[string]bool{},
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
	if e.servableStale(now) && c.refreshing[key] {
		return e, false, true
	}
	if !e.servableStale(now) {
		c.removeLocked(el)
	}
	return nil, false, false
}

// beginRefresh marks key as being refetched; false if another request is.
func (c *ResponseCache) beginRefresh(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshing[key] {
		return false
	}
	c.refreshing[key] = true
	return true
}

func (c *ResponseCache) endRefresh(key string) {
	c.mu.Lock()
	delete(c.refreshing, key)
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
		if e.host == host && purgeMatches(scope, value, e.uri) {
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
	out := CacheStats{Hits: d.hits, Misses: d.misses, Bypasses: d.bypasses, Stale: d.stale, Entries: d.entries, Bytes: d.bytes}
	nowMinute := c.now().Unix() / 60
	for i := int64(cacheSeriesBuckets - 1); i >= 0; i-- {
		m := nowMinute - i
		b := d.series[m%cacheSeriesBuckets]
		if b.Minute != m {
			b = CacheBucket{Minute: m}
		}
		out.Series = append(out.Series, b)
	}
	return out
}

// UsedBytes is the current total size of every entry.
func (c *ResponseCache) UsedBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}
