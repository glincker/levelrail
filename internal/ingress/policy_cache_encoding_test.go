package ingress

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func TestNegotiatedEncoding(t *testing.T) {
	prefer := []string{"zstd", "gzip"}
	tests := []struct {
		name   string
		header []string
		want   string
	}{
		{"absent", nil, "identity"},
		{"empty", []string{""}, "identity"},
		{"identity only", []string{"identity"}, "identity"},
		{"chrome", []string{"gzip, deflate, br, zstd"}, "zstd"},
		{"firefox", []string{"gzip, deflate, br, zstd"}, "zstd"},
		{"safari", []string{"gzip, deflate, br"}, "gzip"},
		{"old chrome", []string{"gzip, deflate, br"}, "gzip"},
		{"curl", []string{"deflate, gzip"}, "gzip"},
		{"order does not matter", []string{"zstd, gzip"}, "zstd"},
		{"reversed order", []string{"gzip, zstd"}, "zstd"},
		{"spacing and case", []string{" GZip ,BR"}, "gzip"},
		{"q prefers gzip", []string{"zstd;q=0.5, gzip;q=0.9"}, "gzip"},
		{"q ties follow server preference", []string{"gzip;q=0.8, zstd;q=0.8"}, "zstd"},
		{"q zero excludes", []string{"zstd;q=0, gzip"}, "gzip"},
		{"all zero", []string{"gzip;q=0, zstd;q=0"}, "identity"},
		{"wildcard", []string{"*"}, "zstd"},
		{"wildcard minus zstd", []string{"zstd;q=0, *"}, "gzip"},
		{"wildcard zero", []string{"*;q=0"}, "identity"},
		{"br only goes to the br variant", []string{"br"}, "br"},
		{"br with q after gzip q", []string{"br;q=1, gzip;q=0.1"}, "gzip"},
		{"deflate only", []string{"deflate"}, "identity"},
		{"malformed q counts as one", []string{"gzip;q=abc"}, "gzip"},
		{"multiple header lines", []string{"deflate", "gzip"}, "gzip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiatedEncoding(tt.header, prefer); got != tt.want {
				t.Fatalf("negotiatedEncoding(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestAcceptsContentEncoding(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		ce     string
		want   bool
	}{
		{"identity always", nil, "", true},
		{"identity label", []string{"gzip"}, "identity", true},
		{"gzip accepted", []string{"deflate, gzip;q=0.5"}, "gzip", true},
		{"gzip refused by q zero", []string{"gzip;q=0"}, "gzip", false},
		{"br to gzip only client", []string{"gzip"}, "br", false},
		{"br to no header", nil, "br", false},
		{"br via wildcard", []string{"*"}, "br", true},
		{"case insensitive", []string{"BR"}, "br", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acceptsContentEncoding(tt.header, tt.ce); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestCacheKeyEncodingVariants(t *testing.T) {
	key := func(ae string, rule trafficpolicy.CacheRule) string {
		r := httptest.NewRequest("GET", "http://a.test/x", nil)
		if ae != "" {
			r.Header.Set("Accept-Encoding", ae)
		}
		k, _ := cacheKey("a.test", r, rule)
		return k
	}
	var none trafficpolicy.CacheRule
	same := []string{"gzip, deflate, br, zstd", "zstd, gzip", "gzip, zstd", "br, zstd;q=0.9, gzip;q=0.8", "*"}
	for _, ae := range same[1:] {
		if key(ae, none) != key(same[0], none) {
			t.Errorf("%q and %q should share a key", ae, same[0])
		}
	}
	if key("gzip, deflate, br", none) == key("gzip, deflate, br, zstd", none) {
		t.Error("gzip-only and zstd clients must not share a key")
	}
	if key("", none) != key("identity", none) || key("", none) == key("gzip", none) {
		t.Error("identity variants")
	}
	if key("gzip", trafficpolicy.CacheRule{Vary: []string{"Accept-Encoding"}}) == key("gzip, deflate", trafficpolicy.CacheRule{Vary: []string{"Accept-Encoding"}}) {
		t.Error("an explicit Vary: Accept-Encoding stays keyed raw")
	}
}

func TestCacheModule_OneEntryAcrossBrowserStrings(t *testing.T) {
	h := newCacheHarness(allPaths)
	h.do(t, "GET", "a.test", "/x", map[string]string{"Accept-Encoding": "gzip, deflate, br, zstd"})
	for _, ae := range []string{"zstd, gzip", "gzip, zstd, br", "*"} {
		if rec := h.do(t, "GET", "a.test", "/x", map[string]string{"Accept-Encoding": ae}); rec.Header().Get(CacheStatusHeader) != cacheHit {
			t.Fatalf("%q = %q, want HIT", ae, rec.Header().Get(CacheStatusHeader))
		}
	}
	if h.calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", h.calls.Load())
	}
}

func TestCacheModule_NeverServesEncodedBodyToClientThatCannotDecode(t *testing.T) {
	h := newCacheHarness(allPaths)
	h.resp = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		if ae := r.Header.Get("Accept-Encoding"); ae == "br" {
			w.Header().Set("Content-Encoding", "br")
			_, _ = w.Write([]byte("brotli-bytes"))
			return
		}
		_, _ = w.Write([]byte("plain"))
	}
	h.do(t, "GET", "a.test", "/x", map[string]string{"Accept-Encoding": "br"})
	if rec := h.do(t, "GET", "a.test", "/x", map[string]string{"Accept-Encoding": "br"}); rec.Header().Get(CacheStatusHeader) != cacheHit || rec.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("br client second request = %q", rec.Header().Get(CacheStatusHeader))
	}
	for _, ae := range []string{"", "identity", "gzip"} {
		rec := h.do(t, "GET", "a.test", "/x", map[string]string{"Accept-Encoding": ae})
		if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != "plain" {
			t.Fatalf("Accept-Encoding %q got Content-Encoding %q body %q", ae, rec.Header().Get("Content-Encoding"), rec.Body.String())
		}
	}
}

func TestCacheModule_GzipBodyNotServedToIdentityClient(t *testing.T) {
	h := newCacheHarness(allPaths)
	h.resp = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		if r.Header.Get("Accept-Encoding") != "" {
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write([]byte("gz"))
			return
		}
		_, _ = w.Write([]byte("plain"))
	}
	h.do(t, "GET", "a.test", "/x", map[string]string{"Accept-Encoding": "gzip"})
	if rec := h.do(t, "GET", "a.test", "/x", nil); rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != "plain" {
		t.Fatalf("identity client got %q %q", rec.Header().Get("Content-Encoding"), rec.Body.String())
	}
}
