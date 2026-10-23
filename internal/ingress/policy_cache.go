package ingress

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
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
		return ReasonAuthorization
	case r.Header.Get("Cookie") != "" && !rule.CacheWithCookies:
		return ReasonCookieRequest
	case r.Header.Get("Range") != "":
		return ReasonRange
	case r.Header.Get("Upgrade") != "":
		return ReasonUpgrade
	case hasDirective(r.Header.Values("Cache-Control"), "no-store"):
		return ReasonNoStore
	}
	return ""
}

// cacheKey never mixes hosts: the request host leads the key, then the
// path and query (minus ignored params), every Vary header (plus
// Accept-Encoding, which upstreams compress by) and any keyed cookies.
// It also returns the normalized URI the entry is stored and purged under.
func cacheKey(host string, r *http.Request, rule trafficpolicy.CacheRule) (key, uri string) {
	uri = normalizedURI(r, rule.IgnoreQueryParams)
	var b strings.Builder
	b.WriteString(host)
	b.WriteByte('\n')
	b.WriteString(uri)
	for _, h := range append([]string{"Accept-Encoding"}, rule.Vary...) {
		b.WriteByte('\n')
		b.WriteString(http.CanonicalHeaderKey(h))
		b.WriteByte('=')
		b.WriteString(strings.Join(r.Header.Values(h), ","))
	}
	for _, name := range rule.KeyCookies {
		b.WriteString("\ncookie:")
		b.WriteString(name)
		b.WriteByte('=')
		if c, err := r.Cookie(name); err == nil {
			b.WriteString(c.Value)
		}
	}
	return b.String(), uri
}

func normalizedURI(r *http.Request, ignore []string) string {
	if len(ignore) == 0 || r.URL.RawQuery == "" {
		return r.URL.RequestURI()
	}
	var kept []string
	for _, part := range strings.Split(r.URL.RawQuery, "&") {
		name, _, _ := strings.Cut(part, "=")
		if n, err := url.QueryUnescape(name); err == nil {
			name = n
		}
		if !slices.Contains(ignore, name) {
			kept = append(kept, part)
		}
	}
	if len(kept) == 0 {
		return r.URL.EscapedPath()
	}
	return r.URL.EscapedPath() + "?" + strings.Join(kept, "&")
}

func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(host)
}

func parseTags(values []string) []string {
	var tags []string
	for _, v := range values {
		for _, t := range strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ' ' }) {
			if len(t) <= maxCacheTagLen && len(tags) < maxCacheTags && !slices.Contains(tags, t) {
				tags = append(tags, t)
			}
		}
	}
	return tags
}

func (m *cacheModule) waitForFetch(r *http.Request, ch <-chan struct{}) {
	t := time.NewTimer(envDuration(EnvCacheCoalesceWait, defaultCacheCoalesceWait))
	defer t.Stop()
	select {
	case <-ch:
	case <-r.Context().Done():
	case <-t.C:
	}
}

func (m *cacheModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	rule, ok := m.rule(r)
	if !ok || rule.Never {
		return next.ServeHTTP(w, r)
	}
	host := requestHost(r)
	if reason := bypassReason(r, rule); reason != "" {
		m.store.record(host, cacheBypass)
		m.store.recordReasons(host, reason)
		w.Header().Set(CacheStatusHeader, cacheBypass)
		return next.ServeHTTP(w, r)
	}
	key, uri := cacheKey(host, r, rule)
	noCache := hasDirective(r.Header.Values("Cache-Control"), "no-cache") || r.Header.Get("Pragma") == "no-cache"
	leader, waited := false, false
	for {
		if !noCache {
			if e, fresh, stale := m.store.lookup(key); e != nil {
				outcome := cacheHit
				if stale && !fresh {
					outcome = cacheStale
				}
				m.serveCached(w, r, host, e, outcome)
				return nil
			}
		}
		if leader = m.store.beginRefresh(key); leader || noCache || waited {
			break
		}
		if ch := m.store.inflight(key); ch != nil {
			m.waitForFetch(r, ch)
		}
		waited = true
	}
	if leader {
		defer m.store.endRefresh(key)
	}
	m.store.record(host, cacheMiss)

	now := m.store.now()
	var old *cacheEntry
	if e := m.store.peek(key); e != nil && !e.fresh(now) {
		old = e
	}
	guard := old != nil && old.servableOnError(now)
	revalidate := old != nil && old.hasValidator() && r.Header.Get("If-None-Match") == "" && r.Header.Get("If-Modified-Since") == ""
	if revalidate {
		if etag := old.header.Get("Etag"); etag != "" {
			r.Header.Set("If-None-Match", etag)
		}
		if lm := old.header.Get("Last-Modified"); lm != "" {
			r.Header.Set("If-Modified-Since", lm)
		}
	}

	rec := &cacheRecorder{ResponseWriterWrapper: &caddyhttp.ResponseWriterWrapper{ResponseWriter: w}, max: m.MaxObjectBytes, hold5xx: guard, hold304: revalidate}
	err := next.ServeHTTP(rec, r)
	if revalidate {
		r.Header.Del("If-None-Match")
		r.Header.Del("If-Modified-Since")
	}
	if rec.held && rec.status == http.StatusNotModified && revalidate {
		m.refreshFromNotModified(rule, old, rec.Header())
		clearHeader(w.Header())
		m.serveCached(w, r, host, old, cacheHit)
		return nil
	}
	if (rec.held || (err != nil && rec.status == 0)) && guard {
		clearHeader(w.Header())
		m.serveCached(w, r, host, old, cacheStale)
		return nil
	}
	if err == nil && rec.status == 0 {
		// Nothing written: the server sends an implicit empty 200.
		rec.Header().Set(CacheStatusHeader, cacheMiss)
		rec.status = http.StatusOK
	}
	m.store.recordBytes(host, false, rec.total)
	if rec.tooLarge {
		m.store.recordReasons(host, ReasonTooLarge)
	}
	if err != nil || r.Method != http.MethodGet || rec.skip {
		return err
	}
	ttl, stale, storable, why := m.freshness(rule, rec.status, rec.Header())
	if !storable {
		m.store.recordReasons(host, why...)
	}
	if storable {
		stored := m.store.now()
		m.store.put(&cacheEntry{
			key: key, host: host, uri: uri, origURI: r.URL.RequestURI(), status: rec.status,
			tags:   parseTags(rec.Header().Values(cacheTagHeader())),
			header: storedHeader(rec.Header()), body: rec.buf.Bytes(),
			stored: stored, expires: stored.Add(ttl), stale: stale,
			onError: time.Duration(rule.StaleIfErrorSeconds) * time.Second,
		})
	}
	return nil
}

func clearHeader(h http.Header) {
	for k := range h {
		delete(h, k)
	}
}

func (m *cacheModule) serveCached(w http.ResponseWriter, r *http.Request, host string, e *cacheEntry, outcome string) {
	m.store.record(host, outcome)
	n := serveEntry(w, r, e, outcome, m.store.now())
	m.store.recordBytes(host, true, n)
}

// refreshFromNotModified extends old's life after the app confirmed it with 304.
func (m *cacheModule) refreshFromNotModified(rule trafficpolicy.CacheRule, old *cacheEntry, fresh http.Header) {
	h := old.header.Clone()
	for _, k := range []string{"Cache-Control", "Expires", "Etag", "Last-Modified"} {
		if v := fresh.Values(k); len(v) > 0 {
			h[k] = slices.Clone(v)
		}
	}
	ttl, stale, ok, _ := m.freshness(rule, old.status, h)
	if !ok {
		return
	}
	now := m.store.now()
	next := *old
	next.header, next.stored, next.expires, next.stale = h, now, now.Add(ttl), stale
	m.store.put(&next)
}

// freshness decides whether a response may be stored and for how long.
// Set-Cookie, private, no-store, no-cache and Vary: * are never stored;
// reasons names why (see Reason*), one per offending Vary header.
func (m *cacheModule) freshness(rule trafficpolicy.CacheRule, status int, h http.Header) (ttl, stale time.Duration, storable bool, reasons []string) {
	if !slices.Contains(rule.EffectiveStatusCodes(), status) {
		return 0, 0, false, []string{ReasonStatusNotCached}
	}
	if h.Get("Set-Cookie") != "" {
		return 0, 0, false, []string{ReasonSetCookie}
	}
	if h.Get("Content-Range") != "" {
		return 0, 0, false, []string{ReasonRange}
	}
	cc := h.Values("Cache-Control")
	switch {
	case hasDirective(cc, "no-store"):
		return 0, 0, false, []string{ReasonNoStore}
	case hasDirective(cc, "private"):
		return 0, 0, false, []string{ReasonPrivate}
	case hasDirective(cc, "no-cache"):
		return 0, 0, false, []string{ReasonNoCache}
	}
	for _, v := range h.Values("Vary") {
		for _, name := range strings.Split(v, ",") {
			name = http.CanonicalHeaderKey(strings.TrimSpace(name))
			if name == "*" || (name != "" && name != "Accept-Encoding" && !slices.ContainsFunc(rule.Vary, func(s string) bool { return http.CanonicalHeaderKey(s) == name })) {
				reasons = append(reasons, ReasonVaryHeader+reasonSep+name)
			}
		}
	}
	if len(reasons) > 0 {
		return 0, 0, false, reasons
	}
	ttl = time.Duration(rule.TTLSeconds) * time.Second
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
	stale = time.Duration(rule.StaleWhileRevalidateSeconds) * time.Second
	return ttl, stale, ttl > 0, nil
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

func serveEntry(w http.ResponseWriter, r *http.Request, e *cacheEntry, outcome string, now time.Time) int64 {
	dst := w.Header()
	for k, v := range e.header {
		dst[k] = slices.Clone(v)
	}
	dst.Set(CacheStatusHeader, outcome)
	dst.Set("Age", strconv.Itoa(int(now.Sub(e.stored).Seconds())))
	if notModified(r, e) {
		w.WriteHeader(http.StatusNotModified)
		return 0
	}
	w.WriteHeader(e.status)
	if r.Method == http.MethodHead {
		return 0
	}
	n, _ := w.Write(e.body)
	return int64(n)
}

func notModified(r *http.Request, e *cacheEntry) bool {
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		etag := e.header.Get("Etag")
		return etag != "" && inm == etag
	}
	ims, err := http.ParseTime(r.Header.Get("If-Modified-Since"))
	if err != nil {
		return false
	}
	lm, err := http.ParseTime(e.header.Get("Last-Modified"))
	return err == nil && !lm.After(ims)
}

// cacheRecorder streams the response to the client while keeping a copy
// up to max bytes; a larger or streaming response is passed through only.
type cacheRecorder struct {
	*caddyhttp.ResponseWriterWrapper
	max    int64
	status int
	buf    bytes.Buffer
	skip   bool
	// tooLarge is set when the body exceeded max, as opposed to other skips.
	tooLarge bool
	total    int64
	// hold5xx and hold304 swallow that response so the handler can answer
	// from a stored copy instead; held records that it happened.
	hold5xx, hold304 bool
	held             bool
}

func (c *cacheRecorder) WriteHeader(code int) {
	if c.status == 0 && code >= 200 {
		c.status = code
		if (c.hold5xx && code >= 500) || (c.hold304 && code == http.StatusNotModified) {
			c.held = true
			return
		}
		c.Header().Set(CacheStatusHeader, cacheMiss)
		if strings.HasPrefix(c.Header().Get("Content-Type"), "text/event-stream") {
			c.skip = true
		}
		if n, err := strconv.ParseInt(c.Header().Get("Content-Length"), 10, 64); err == nil && n > c.max {
			c.skip, c.tooLarge = true, true
		}
	}
	c.ResponseWriterWrapper.WriteHeader(code)
}

func (c *cacheRecorder) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.WriteHeader(http.StatusOK)
	}
	if c.held {
		return len(p), nil
	}
	c.total += int64(len(p))
	if !c.skip {
		if int64(c.buf.Len()+len(p)) > c.max {
			c.skip, c.tooLarge = true, true
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

func (c *cacheRecorder) Flush() {
	if c.held {
		return
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
