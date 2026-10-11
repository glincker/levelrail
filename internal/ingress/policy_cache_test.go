package ingress

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func caddyCtx() caddy.Context { return caddy.Context{} }

type cacheHarness struct {
	m     *cacheModule
	store *ResponseCache
	now   time.Time
	calls atomic.Int32
	resp  func(w http.ResponseWriter, r *http.Request)
}

func newCacheHarness(rules ...trafficpolicy.CacheRule) *cacheHarness {
	h := &cacheHarness{store: NewResponseCache(1 << 20), now: time.Unix(1_800_000_000, 0)}
	h.store.now = func() time.Time { return h.now }
	h.m = &cacheModule{Rules: rules, MaxObjectBytes: 1024, MaxTTLSeconds: 3600, store: h.store}
	h.resp = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = fmt.Fprintf(w, "body %s", r.URL.RequestURI()) //nolint:gosec // test upstream echoes the path
	}
	return h
}

func (h *cacheHarness) do(t *testing.T, method, host, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://"+host+target, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		h.calls.Add(1)
		h.resp(w, r)
		return nil
	})
	if err := h.m.ServeHTTP(rec, r, next); err != nil {
		t.Fatal(err)
	}
	return rec
}

var allPaths = trafficpolicy.CacheRule{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/"}, TTLSeconds: 30}

func TestCacheModule_Behaviour(t *testing.T) {
	tests := []struct {
		name    string
		rule    trafficpolicy.CacheRule
		resp    func(w http.ResponseWriter, r *http.Request)
		first   map[string]string
		second  map[string]string
		want    []string
		upstrem int32
	}{
		{"miss then hit", allPaths, nil, nil, nil, []string{"MISS", "HIT"}, 1},
		{"cookie bypasses", allPaths, nil, map[string]string{"Cookie": "a=1"}, map[string]string{"Cookie": "a=1"}, []string{"BYPASS", "BYPASS"}, 2},
		{"cookie allowed when opted in", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, CacheWithCookies: true}, nil, map[string]string{"Cookie": "a=1"}, map[string]string{"Cookie": "b=2"}, []string{"MISS", "HIT"}, 1},
		{"authorization bypasses", allPaths, nil, map[string]string{"Authorization": "Bearer x"}, nil, []string{"BYPASS", "MISS"}, 2},
		{"range bypasses", allPaths, nil, map[string]string{"Range": "bytes=0-1"}, nil, []string{"BYPASS", "MISS"}, 2},
		{"set-cookie never stored", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Set-Cookie", "s=1")
			_, _ = w.Write([]byte("x"))
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"private never stored", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "private, max-age=60")
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"no-store never stored", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"max-age 0 not stored", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "max-age=0")
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"override ignores upstream max-age 0", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, OverrideUpstream: true}, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "max-age=0")
		}, nil, nil, []string{"MISS", "HIT"}, 1},
		{"500 not stored", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(500)
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"unknown vary not stored", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Vary", "User-Agent")
			w.Header().Set("Cache-Control", "max-age=60")
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"vary splits key", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, Vary: []string{"Accept-Language"}}, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Vary", "Accept-Language")
			w.Header().Set("Cache-Control", "max-age=60")
		}, map[string]string{"Accept-Language": "en"}, map[string]string{"Accept-Language": "fr"}, []string{"MISS", "MISS"}, 2},
		{"accept-encoding splits key", allPaths, nil, map[string]string{"Accept-Encoding": "gzip"}, nil, []string{"MISS", "MISS"}, 2},
		{"oversized body passes through", allPaths, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "max-age=60")
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		}, nil, nil, []string{"MISS", "MISS"}, 2},
		{"client no-cache refetches", allPaths, nil, nil, map[string]string{"Cache-Control": "no-cache"}, []string{"MISS", "MISS"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCacheHarness(tt.rule)
			if tt.resp != nil {
				h.resp = tt.resp
			}
			a := h.do(t, "GET", "app.test", "/p?q=1", tt.first)
			b := h.do(t, "GET", "app.test", "/p?q=1", tt.second)
			got := []string{a.Header().Get(CacheStatusHeader), b.Header().Get(CacheStatusHeader)}
			if got[0] != tt.want[0] || got[1] != tt.want[1] || h.calls.Load() != tt.upstrem {
				t.Fatalf("X-Cache = %v upstream calls %d, want %v and %d", got, h.calls.Load(), tt.want, tt.upstrem)
			}
			if b.Body.String() != a.Body.String() {
				t.Fatalf("bodies differ: %q vs %q", a.Body.String(), b.Body.String())
			}
		})
	}
}

func TestCacheModule_HostIsolationHeadAndScope(t *testing.T) {
	h := newCacheHarness(trafficpolicy.CacheRule{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/assets"}, TTLSeconds: 30})
	h.do(t, "GET", "a.test", "/assets/x", nil)
	if rec := h.do(t, "GET", "b.test", "/assets/x", nil); rec.Header().Get(CacheStatusHeader) != cacheMiss {
		t.Fatal("cache served across hosts")
	}
	if rec := h.do(t, "HEAD", "a.test", "/assets/x", nil); rec.Header().Get(CacheStatusHeader) != cacheHit || rec.Body.Len() != 0 {
		t.Fatalf("HEAD = %q body %d", rec.Header().Get(CacheStatusHeader), rec.Body.Len())
	}
	if rec := h.do(t, "GET", "a.test", "/other", nil); rec.Header().Get(CacheStatusHeader) != "" {
		t.Fatal("uncovered path got an X-Cache header")
	}
	if rec := h.do(t, "POST", "a.test", "/assets/x", nil); rec.Header().Get(CacheStatusHeader) != "" {
		t.Fatal("POST touched the cache")
	}
}

func TestCacheModule_TTLStaleAndPurge(t *testing.T) {
	h := newCacheHarness(trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 10, OverrideUpstream: true, StaleWhileRevalidateSeconds: 30})
	h.do(t, "GET", "a.test", "/x", nil)
	h.now = h.now.Add(5 * time.Second)
	if rec := h.do(t, "GET", "a.test", "/x", nil); rec.Header().Get(CacheStatusHeader) != cacheHit || rec.Header().Get("Age") != "5" {
		t.Fatalf("fresh = %q age %q", rec.Header().Get(CacheStatusHeader), rec.Header().Get("Age"))
	}
	h.now = h.now.Add(10 * time.Second)
	key := cacheKey("a.test", httptest.NewRequest("GET", "http://a.test/x", nil), nil)
	h.store.beginRefresh(key)
	if rec := h.do(t, "GET", "a.test", "/x", nil); rec.Header().Get(CacheStatusHeader) != cacheStale {
		t.Fatalf("expired while refreshing = %q, want STALE", rec.Header().Get(CacheStatusHeader))
	}
	h.store.endRefresh(key)
	if rec := h.do(t, "GET", "a.test", "/x", nil); rec.Header().Get(CacheStatusHeader) != cacheMiss {
		t.Fatalf("expired, nobody refreshing = %q, want MISS", rec.Header().Get(CacheStatusHeader))
	}
	h.now = h.now.Add(time.Hour)
	if rec := h.do(t, "GET", "a.test", "/x", nil); rec.Header().Get(CacheStatusHeader) != cacheMiss {
		t.Fatal("entry past its stale window was served")
	}

	for _, p := range []string{"/a/1", "/a/2", "/b/1"} {
		h.do(t, "GET", "a.test", p, nil)
	}
	h.do(t, "GET", "c.test", "/a/1", nil)
	if n := h.store.Purge("a.test", PurgeURL, "/a/1"); n != 1 {
		t.Fatalf("purge url = %d", n)
	}
	if n := h.store.Purge("A.TEST", PurgePrefix, "/a"); n != 1 {
		t.Fatalf("purge prefix = %d", n)
	}
	if n := h.store.Purge("a.test", PurgeAll, ""); n != 2 {
		t.Fatalf("purge all = %d", n)
	}
	if s := h.store.Stats("c.test"); s.Entries != 1 {
		t.Fatalf("other host lost entries on purge: %+v", s)
	}
	s := h.store.Stats("a.test")
	if s.Hits == 0 || s.Misses == 0 || s.Stale != 1 || s.Entries != 0 || s.Bytes != 0 || len(s.Series) != cacheSeriesBuckets {
		t.Fatalf("Stats() = %+v", s)
	}
}

func TestResponseCache_ByteCapEviction(t *testing.T) {
	c := NewResponseCache(4000)
	now := time.Unix(1_800_000_000, 0)
	put := func(k string) {
		c.put(&cacheEntry{key: k, host: "a.test", uri: "/" + k, status: 200, body: make([]byte, 1000), stored: now, expires: now.Add(time.Hour)})
	}
	put("1")
	put("2")
	put("3")
	if e, fresh, _ := c.lookup("1"); e == nil || !fresh {
		t.Fatal("entry 1 missing")
	}
	put("4")
	if e, _, _ := c.lookup("2"); e != nil {
		t.Fatal("least recently used entry 2 survived eviction")
	}
	if e, _, _ := c.lookup("1"); e == nil {
		t.Fatal("recently used entry 1 was evicted")
	}
	if c.UsedBytes() > c.MaxBytes() {
		t.Fatalf("used %d > cap %d", c.UsedBytes(), c.MaxBytes())
	}
	c.put(&cacheEntry{key: "huge", host: "a.test", body: make([]byte, 5000)})
	if e, _, _ := c.lookup("huge"); e != nil {
		t.Fatal("entry larger than the whole cache was stored")
	}
}

func TestCacheModule_Concurrent(t *testing.T) {
	h := newCacheHarness(allPaths)
	h.store = NewResponseCache(64 << 10)
	h.m.store = h.store
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				h.do(t, "GET", "a.test", fmt.Sprintf("/p/%d", (i+j)%20), nil)
				if j%10 == 0 {
					h.store.Purge("a.test", PurgePrefix, "/p/1")
				}
				_ = h.store.Stats("a.test")
			}
		}(i)
	}
	wg.Wait()
	if h.store.UsedBytes() > h.store.MaxBytes() {
		t.Fatalf("used %d exceeds cap", h.store.UsedBytes())
	}
}

func TestDirectives(t *testing.T) {
	cc := []string{`public, s-maxage="120"`, "max-age=60"}
	if !hasDirective(cc, "PUBLIC") || hasDirective(cc, "private") {
		t.Fatal("hasDirective")
	}
	if d, ok := directiveSeconds(cc, "s-maxage"); !ok || d != 120*time.Second {
		t.Fatalf("s-maxage = %v %v", d, ok)
	}
	if _, ok := directiveSeconds([]string{"max-age=-1"}, "max-age"); ok {
		t.Fatal("negative max-age accepted")
	}
}
