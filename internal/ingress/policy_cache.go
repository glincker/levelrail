package ingress

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func init() {
	caddy.RegisterModule(cacheModule{})
}

const (
	cacheHit    = "HIT"
	cacheMiss   = "MISS"
	cacheBypass = "BYPASS"
	cacheStale  = "STALE"

	// CacheStatusHeader reports HIT, MISS, STALE or BYPASS on cacheable paths.
	CacheStatusHeader = "X-Cache"
)

// CacheHandler is the wire shape of the levelrail_cache handler.
type CacheHandler struct {
	Handler        string                    `json:"handler"`
	Key            string                    `json:"key"`
	Rules          []trafficpolicy.CacheRule `json:"rules"`
	MaxObjectBytes int64                     `json:"max_object_bytes"`
	MaxTTLSeconds  int                       `json:"max_ttl_seconds"`
}

func newCacheHandler(host string, c trafficpolicy.Cache, l trafficpolicy.Limits) CacheHandler {
	maxObject := c.MaxObjectBytes
	if maxObject <= 0 || maxObject > l.CacheMaxObject {
		maxObject = l.CacheMaxObject
	}
	return CacheHandler{
		Handler: "levelrail_cache", Key: strings.ToLower(host), Rules: c.Rules,
		MaxObjectBytes: maxObject, MaxTTLSeconds: int(l.CacheMaxTTL.Seconds()),
	}
}

// cacheModule (http.handlers.levelrail_cache) serves and stores whole
// responses in DefaultResponseCache.
type cacheModule struct {
	Key            string                    `json:"key"`
	Rules          []trafficpolicy.CacheRule `json:"rules"`
	MaxObjectBytes int64                     `json:"max_object_bytes"`
	MaxTTLSeconds  int                       `json:"max_ttl_seconds"`
	store          *ResponseCache
}

func (cacheModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_cache",
		New: func() caddy.Module { return new(cacheModule) },
	}
}

// Provision binds the module to the process-wide cache.
func (m *cacheModule) Provision(caddy.Context) error {
	if m.store == nil {
		m.store = DefaultResponseCache()
	}
	return nil
}

func (m *cacheModule) rule(r *http.Request) (trafficpolicy.CacheRule, bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return trafficpolicy.CacheRule{}, false
	}
	for _, rule := range m.Rules {
		if rule.Match.Matches(r.Method, r.URL.Path) {
			return rule, true
		}
	}
	return trafficpolicy.CacheRule{}, false
}

// bypassReason returns why r must not be served from or stored in the
// cache: credentials, cookies, ranges, upgrades and explicit no-store.
func bypassReason(r *http.Request, rule trafficpolicy.CacheRule) string {
	switch {
	case r.Header.Get("Authorization") != "":
		return "authorization"
	case r.Header.Get("Cookie") != "" && !rule.CacheWithCookies:
		return "cookie"
	case r.Header.Get("Range") != "":
		return "range"
	case r.Header.Get("Upgrade") != "":
		return "upgrade"
	case hasDirective(r.Header.Values("Cache-Control"), "no-store"):
		return "no-store"
	}
	return ""
}

// cacheKey never mixes hosts: the request host leads the key, and every
// Vary header (plus Accept-Encoding, which upstreams compress by) follows.
func cacheKey(host string, r *http.Request, vary []string) string {
	var b strings.Builder
	b.WriteString(host)
	b.WriteByte('\n')
	b.WriteString(r.URL.RequestURI())
	for _, h := range append([]string{"Accept-Encoding"}, vary...) {
		b.WriteByte('\n')
		b.WriteString(http.CanonicalHeaderKey(h))
		b.WriteByte('=')
		b.WriteString(strings.Join(r.Header.Values(h), ","))
	}
	return b.String()
}

func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

func (m *cacheModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	rule, ok := m.rule(r)
	if !ok {
		return next.ServeHTTP(w, r)
	}
	host := requestHost(r)
	if reason := bypassReason(r, rule); reason != "" {
		m.store.record(host, cacheBypass)
		w.Header().Set(CacheStatusHeader, cacheBypass)
		return next.ServeHTTP(w, r)
	}
	key := cacheKey(host, r, rule.Vary)
	noCache := hasDirective(r.Header.Values("Cache-Control"), "no-cache") || r.Header.Get("Pragma") == "no-cache"
	if !noCache {
		if e, fresh, stale := m.store.lookup(key); e != nil {
			outcome := cacheHit
			if stale && !fresh {
				outcome = cacheStale
			}
			m.store.record(host, outcome)
			serveEntry(w, r, e, outcome, m.store.now())
			return nil
		}
	}
	m.store.record(host, cacheMiss)
	refreshing := m.store.beginRefresh(key)
	if refreshing {
		defer m.store.endRefresh(key)
	}
	rec := &cacheRecorder{ResponseWriterWrapper: &caddyhttp.ResponseWriterWrapper{ResponseWriter: w}, max: m.MaxObjectBytes}
	err := next.ServeHTTP(rec, r)
	if err == nil && rec.status == 0 {
		// Nothing written: the server sends an implicit empty 200.
		rec.Header().Set(CacheStatusHeader, cacheMiss)
		rec.status = http.StatusOK
	}
	if err != nil || r.Method != http.MethodGet || rec.skip {
		return err
	}
	ttl, stale, storable := m.freshness(rule, rec.status, rec.Header())
	if storable {
		now := m.store.now()
		m.store.put(&cacheEntry{
			key: key, host: host, uri: r.URL.RequestURI(), status: rec.status,
			header: storedHeader(rec.Header()), body: rec.buf.Bytes(),
			stored: now, expires: now.Add(ttl), stale: stale,
		})
	}
	return nil
}

// freshness decides whether a response may be stored and for how long.
// Set-Cookie, private, no-store, no-cache and Vary: * are never stored.
func (m *cacheModule) freshness(rule trafficpolicy.CacheRule, status int, h http.Header) (time.Duration, time.Duration, bool) {
	if !slices.Contains(rule.EffectiveStatusCodes(), status) || h.Get("Set-Cookie") != "" || h.Get("Content-Range") != "" {
		return 0, 0, false
	}
	cc := h.Values("Cache-Control")
	if hasDirective(cc, "no-store") || hasDirective(cc, "private") || hasDirective(cc, "no-cache") {
		return 0, 0, false
	}
	for _, v := range h.Values("Vary") {
		for _, name := range strings.Split(v, ",") {
			name = http.CanonicalHeaderKey(strings.TrimSpace(name))
			if name == "*" || (name != "" && name != "Accept-Encoding" && !slices.ContainsFunc(rule.Vary, func(s string) bool { return http.CanonicalHeaderKey(s) == name })) {
				return 0, 0, false
			}
		}
	}
	ttl := time.Duration(rule.TTLSeconds) * time.Second
	if !rule.OverrideUpstream {
		if s, ok := directiveSeconds(cc, "s-maxage"); ok {
			ttl = s
		} else if s, ok := directiveSeconds(cc, "max-age"); ok {
			ttl = s
		}
	}
	if maxTTL := time.Duration(m.MaxTTLSeconds) * time.Second; maxTTL > 0 && ttl > maxTTL {
		ttl = maxTTL
	}
	stale := time.Duration(rule.StaleWhileRevalidateSeconds) * time.Second
	return ttl, stale, ttl > 0
}

func hasDirective(values []string, directive string) bool {
	for _, v := range values {
		for _, d := range strings.Split(v, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(d), "=")
			if strings.EqualFold(name, directive) {
				return true
			}
		}
	}
	return false
}

func directiveSeconds(values []string, directive string) (time.Duration, bool) {
	for _, v := range values {
		for _, d := range strings.Split(v, ",") {
			name, val, ok := strings.Cut(strings.TrimSpace(d), "=")
			if !ok || !strings.EqualFold(name, directive) {
				continue
			}
			n, err := strconv.Atoi(strings.Trim(val, `"`))
			if err != nil || n < 0 {
				return 0, false
			}
			return time.Duration(n) * time.Second, true
		}
	}
	return 0, false
}

var unstoredHeaders = []string{"Connection", "Keep-Alive", "Transfer-Encoding", "Upgrade", "Trailer", "Te", "Proxy-Connection", CacheStatusHeader, "Age"}

func storedHeader(h http.Header) http.Header {
	out := h.Clone()
	for _, k := range unstoredHeaders {
		out.Del(k)
	}
	return out
}

func serveEntry(w http.ResponseWriter, r *http.Request, e *cacheEntry, outcome string, now time.Time) {
	dst := w.Header()
	for k, v := range e.header {
		dst[k] = slices.Clone(v)
	}
	dst.Set(CacheStatusHeader, outcome)
	dst.Set("Age", strconv.Itoa(int(now.Sub(e.stored).Seconds())))
	if etag := e.header.Get("Etag"); etag != "" && r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(e.status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(e.body)
	}
}

// cacheRecorder streams the response to the client while keeping a copy
// up to max bytes; a larger or streaming response is passed through only.
type cacheRecorder struct {
	*caddyhttp.ResponseWriterWrapper
	max    int64
	status int
	buf    bytes.Buffer
	skip   bool
}

func (c *cacheRecorder) WriteHeader(code int) {
	if c.status == 0 && code >= 200 {
		c.status = code
		c.Header().Set(CacheStatusHeader, cacheMiss)
		if strings.HasPrefix(c.Header().Get("Content-Type"), "text/event-stream") {
			c.skip = true
		}
		if n, err := strconv.ParseInt(c.Header().Get("Content-Length"), 10, 64); err == nil && n > c.max {
			c.skip = true
		}
	}
	c.ResponseWriterWrapper.WriteHeader(code)
}

func (c *cacheRecorder) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.WriteHeader(http.StatusOK)
	}
	if !c.skip {
		if int64(c.buf.Len()+len(p)) > c.max {
			c.skip = true
			c.buf = bytes.Buffer{}
		} else {
			c.buf.Write(p)
		}
	}
	return c.ResponseWriterWrapper.Write(p)
}

func (c *cacheRecorder) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{c}, r)
}

var (
	_ caddyhttp.MiddlewareHandler = (*cacheModule)(nil)
	_ caddy.Provisioner           = (*cacheModule)(nil)
)
