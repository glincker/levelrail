package ingress

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func reasonMap(s CacheStats) map[string]uint64 {
	m := map[string]uint64{}
	for _, r := range s.Reasons {
		k := r.Code
		if r.Detail != "" {
			k += ":" + r.Detail
		}
		m[k] = r.Count
	}
	return m
}

func TestCacheModule_CountsReasons(t *testing.T) {
	tests := []struct {
		name string
		hdr  map[string]string
		resp map[string]string
		code int
		body string
		want map[string]uint64
	}{
		{"next.js vary", nil, map[string]string{"Cache-Control": "s-maxage=60", "Vary": "rsc, next-router-state-tree, Accept-Encoding"}, 200, "x",
			map[string]uint64{"vary_header:Rsc": 1, "vary_header:Next-Router-State-Tree": 1}},
		{"vary star", nil, map[string]string{"Vary": "*"}, 200, "x", map[string]uint64{"vary_header:*": 1}},
		{"set cookie", nil, map[string]string{"Set-Cookie": "a=b"}, 200, "x", map[string]uint64{"set_cookie": 1}},
		{"private", nil, map[string]string{"Cache-Control": "private, max-age=5"}, 200, "x", map[string]uint64{"private": 1}},
		{"no-store", nil, map[string]string{"Cache-Control": "no-store"}, 200, "x", map[string]uint64{"no_store": 1}},
		{"no-cache", nil, map[string]string{"Cache-Control": "no-cache"}, 200, "x", map[string]uint64{"no_cache": 1}},
		{"status", nil, map[string]string{"Cache-Control": "max-age=5"}, 404, "x", map[string]uint64{"status_not_cached": 1}},
		{"too large", nil, map[string]string{"Cache-Control": "max-age=5"}, 200, strings.Repeat("x", 2048), map[string]uint64{"too_large": 1}},
		{"range request", map[string]string{"Range": "bytes=0-1"}, nil, 200, "x", map[string]uint64{"range": 1}},
		{"authorization", map[string]string{"Authorization": "Bearer t"}, nil, 200, "x", map[string]uint64{"authorization": 1}},
		{"cookie request", map[string]string{"Cookie": "a=b"}, nil, 200, "x", map[string]uint64{"cookie_request": 1}},
		{"cacheable", nil, map[string]string{"Cache-Control": "max-age=60"}, 200, "x", map[string]uint64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCacheHarness(allPaths)
			h.resp = func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.resp {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}
			h.do(t, "GET", "a.test", "/p", tt.hdr)
			got := reasonMap(h.store.Stats("a.test"))
			if len(got) != len(tt.want) {
				t.Fatalf("reasons = %v, want %v", got, tt.want)
			}
			for k, n := range tt.want {
				if got[k] != n {
					t.Fatalf("reasons = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestRecordReasons_CapsDistinctKeys(t *testing.T) {
	t.Setenv(EnvCacheReasonKeys, "2")
	c := NewResponseCache(1 << 20)
	c.recordReasons("a.test", "vary_header:A", "vary_header:B", "vary_header:C", "vary_header:C", "vary_header:A")
	got := reasonMap(c.Stats("a.test"))
	want := map[string]uint64{"vary_header:A": 2, "vary_header:B": 1, ReasonOther: 2}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("reasons = %v, want %v", got, want)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("reasons = %v, want %v", got, want)
	}
}

func TestMergeCacheStats_MergesReasons(t *testing.T) {
	a := CacheStats{Reasons: []CacheReason{{Code: "vary_header", Detail: "Rsc", Count: 2}, {Code: "set_cookie", Count: 1}}}
	b := CacheStats{Reasons: []CacheReason{{Code: "vary_header", Detail: "Rsc", Count: 3}}}
	got := MergeCacheStats(a, b).Reasons
	want := []CacheReason{{Code: "vary_header", Detail: "Rsc", Count: 5}, {Code: "set_cookie", Count: 1}}
	if !slices.Equal(got, want) {
		t.Fatalf("merged reasons = %v, want %v", got, want)
	}
}
