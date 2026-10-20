package ingress

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func xcache(h http.Header) string { return h.Get(CacheStatusHeader) }

func newRequest(method, host, target string) *http.Request {
	return httptest.NewRequest(method, "http://"+host+target, nil)
}

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func errNext(err error) caddyhttp.Handler {
	return caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { return err })
}

func TestCacheModule_TagsPurge(t *testing.T) {
	h := newCacheHarness(allPaths)
	h.resp = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		if r.URL.Path == "/a" {
			w.Header().Set("Cache-Tag", "blog, post-1")
		} else {
			w.Header().Set("Cache-Tag", "shop")
		}
		_, _ = w.Write([]byte("x"))
	}
	h.do(t, "GET", "a.test", "/a", nil)
	h.do(t, "GET", "a.test", "/b", nil)
	if n := h.store.Purge("a.test", PurgeTag, "post-1"); n != 1 {
		t.Fatalf("purge tag = %d, want 1", n)
	}
	if got := xcache(h.do(t, "GET", "a.test", "/b", nil).Header()); got != cacheHit {
		t.Fatalf("untagged-for-purge entry = %q, want HIT", got)
	}
	if got := xcache(h.do(t, "GET", "a.test", "/a", nil).Header()); got != cacheMiss {
		t.Fatalf("purged entry = %q, want MISS", got)
	}
	if n := h.store.Purge("a.test", PurgeTag, "nope"); n != 0 {
		t.Fatalf("purge unknown tag = %d", n)
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want int
	}{
		{"comma and space", []string{"a, b c"}, 3},
		{"dedupe", []string{"a,a", "a"}, 1},
		{"empty", nil, 0},
	}
	for _, tt := range tests {
		if got := parseTags(tt.in); len(got) != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
}

func TestCacheModule_KeyShaping(t *testing.T) {
	tests := []struct {
		name   string
		rule   trafficpolicy.CacheRule
		a, b   string
		cookie [2]string
		want   string
	}{
		{"ignored param shares entry", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, IgnoreQueryParams: []string{"utm_source"}}, "/p?id=1&utm_source=x", "/p?utm_source=y&id=1", [2]string{}, cacheHit},
		{"other param splits", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, IgnoreQueryParams: []string{"utm_source"}}, "/p?id=1", "/p?id=2", [2]string{}, cacheMiss},
		{"keyed cookie splits", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, CacheWithCookies: true, KeyCookies: []string{"lang"}}, "/p", "/p", [2]string{"lang=en", "lang=fr"}, cacheMiss},
		{"keyed cookie shares", trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, CacheWithCookies: true, KeyCookies: []string{"lang"}}, "/p", "/p", [2]string{"lang=en; x=1", "lang=en; x=2"}, cacheHit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCacheHarness(tt.rule)
			hdr := func(c string) map[string]string {
				if c == "" {
					return nil
				}
				return map[string]string{"Cookie": c}
			}
			h.do(t, "GET", "a.test", tt.a, hdr(tt.cookie[0]))
			if got := xcache(h.do(t, "GET", "a.test", tt.b, hdr(tt.cookie[1])).Header()); got != tt.want {
				t.Fatalf("second = %q, want %s", got, tt.want)
			}
		})
	}
}

func TestCacheModule_PurgeURLAcceptsOriginalQuery(t *testing.T) {
	h := newCacheHarness(trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 30, IgnoreQueryParams: []string{"utm"}})
	h.do(t, "GET", "a.test", "/p?utm=1", nil)
	if n := h.store.Purge("a.test", PurgeURL, "/p?utm=1"); n != 1 {
		t.Fatalf("purge by original URL = %d", n)
	}
}

func TestCacheModule_NeverRule(t *testing.T) {
	never := trafficpolicy.CacheRule{Match: trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: "/admin"}, Never: true}
	h := newCacheHarness(never, allPaths)
	h.do(t, "GET", "a.test", "/admin/x", nil)
	if got := xcache(h.do(t, "GET", "a.test", "/admin/x", nil).Header()); got != "" || h.calls.Load() != 2 {
		t.Fatalf("never path: header %q calls %d", got, h.calls.Load())
	}
	h.do(t, "GET", "a.test", "/ok", nil)
	if got := xcache(h.do(t, "GET", "a.test", "/ok", nil).Header()); got != cacheHit {
		t.Fatalf("other path = %q", got)
	}
}

func TestCacheModule_StaleIfError(t *testing.T) {
	rule := trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 10, OverrideUpstream: true, StaleIfErrorSeconds: 100}
	tests := []struct {
		name string
		fail func(w http.ResponseWriter, r *http.Request)
		wait time.Duration
		want string
	}{
		{"5xx served stale", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503); _, _ = w.Write([]byte("down")) }, 30 * time.Second, cacheStale},
		{"past the window passes the error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }, 200 * time.Second, cacheMiss},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCacheHarness(rule)
			good := h.resp
			h.do(t, "GET", "a.test", "/x", nil)
			h.now = h.now.Add(tt.wait)
			h.resp = tt.fail
			rec := h.do(t, "GET", "a.test", "/x", nil)
			if got := xcache(rec.Header()); got != tt.want {
				t.Fatalf("X-Cache = %q, want %s", got, tt.want)
			}
			if tt.want == cacheStale && (rec.Code != 200 || rec.Body.String() != "body /x") {
				t.Fatalf("stale body %d %q", rec.Code, rec.Body.String())
			}
			if tt.want == cacheMiss && rec.Code != 503 {
				t.Fatalf("status %d, want the 503 through", rec.Code)
			}
			h.resp = good
		})
	}
}

func TestCacheModule_StaleIfErrorOnHandlerError(t *testing.T) {
	rule := trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 10, OverrideUpstream: true, StaleIfErrorSeconds: 100}
	h := newCacheHarness(rule)
	h.do(t, "GET", "a.test", "/x", nil)
	h.now = h.now.Add(20 * time.Second)
	r := newRequest("GET", "a.test", "/x")
	rec := newRecorder()
	err := h.m.ServeHTTP(rec, r, errNext(fmt.Errorf("dial failed")))
	if err != nil || xcache(rec.Header()) != cacheStale {
		t.Fatalf("err %v X-Cache %q", err, xcache(rec.Header()))
	}
}

func TestCacheModule_Revalidation(t *testing.T) {
	rule := trafficpolicy.CacheRule{Match: allPaths.Match, TTLSeconds: 10, OverrideUpstream: true}
	h := newCacheHarness(rule)
	var sawINM string
	h.resp = func(w http.ResponseWriter, r *http.Request) {
		sawINM = r.Header.Get("If-None-Match")
		w.Header().Set("Etag", `"v1"`)
		if sawINM == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte("payload"))
	}
	h.do(t, "GET", "a.test", "/x", nil)
	h.now = h.now.Add(20 * time.Second)
	rec := h.do(t, "GET", "a.test", "/x", nil)
	if sawINM != `"v1"` || rec.Code != 200 || rec.Body.String() != "payload" || xcache(rec.Header()) != cacheHit {
		t.Fatalf("revalidated: inm %q code %d body %q xcache %q", sawINM, rec.Code, rec.Body.String(), xcache(rec.Header()))
	}
	calls := h.calls.Load()
	if got := xcache(h.do(t, "GET", "a.test", "/x", nil).Header()); got != cacheHit || h.calls.Load() != calls {
		t.Fatalf("after revalidation = %q, upstream calls %d to %d", got, calls, h.calls.Load())
	}
}

func TestCacheModule_IfModifiedSince(t *testing.T) {
	h := newCacheHarness(allPaths)
	lm := time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat)
	h.resp = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("Last-Modified", lm)
		_, _ = w.Write([]byte("x"))
	}
	h.do(t, "GET", "a.test", "/x", nil)
	if rec := h.do(t, "GET", "a.test", "/x", map[string]string{"If-Modified-Since": lm}); rec.Code != http.StatusNotModified {
		t.Fatalf("IMS = %d", rec.Code)
	}
}

func TestCacheModule_CoalescesMisses(t *testing.T) {
	h := newCacheHarness(allPaths)
	release := make(chan struct{})
	h.resp = func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.Header().Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte("x"))
	}
	var wg sync.WaitGroup
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = xcache(h.do(t, "GET", "a.test", "/slow", nil).Header())
		}(i)
	}
	for h.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if h.calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", h.calls.Load())
	}
	miss := 0
	for _, r := range results {
		if r == cacheMiss {
			miss++
		}
	}
	if miss != 1 {
		t.Fatalf("results %v, want exactly one MISS", results)
	}
}

func TestCacheModule_ByteAccounting(t *testing.T) {
	h := newCacheHarness(allPaths)
	h.do(t, "GET", "a.test", "/x", nil)
	h.do(t, "GET", "a.test", "/x", nil)
	h.do(t, "GET", "a.test", "/x", nil)
	s := h.store.Stats("a.test")
	if s.OriginBytes == 0 || s.HitBytes != 2*s.OriginBytes {
		t.Fatalf("origin %d hit %d", s.OriginBytes, s.HitBytes)
	}
	var sum uint64
	for _, b := range s.Series {
		sum += b.HitBytes
	}
	if sum != s.HitBytes {
		t.Fatalf("series hit bytes %d != total %d", sum, s.HitBytes)
	}
}

func TestResponseCache_DeterministicEvictionAndHosts(t *testing.T) {
	c := NewResponseCache(3000)
	now := time.Unix(1_800_000_000, 0)
	for i := 0; i < 10; i++ {
		c.put(&cacheEntry{key: fmt.Sprint(i), host: "b.test", uri: "/", status: 200, body: make([]byte, 800), stored: now, expires: now.Add(time.Hour)})
	}
	if c.UsedBytes() > 3000 {
		t.Fatalf("used %d", c.UsedBytes())
	}
	if e, _, _ := c.lookup("0"); e != nil {
		t.Fatal("oldest survived")
	}
	if e, _, _ := c.lookup("9"); e == nil {
		t.Fatal("newest evicted")
	}
	if hs := c.Hosts(); len(hs) != 1 || hs[0] != "b.test" {
		t.Fatalf("hosts %v", hs)
	}
}
