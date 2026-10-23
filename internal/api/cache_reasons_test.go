package api

import (
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

func TestAppReasons(t *testing.T) {
	rsc := ingress.CacheReason{Code: ingress.ReasonVaryHeader, Detail: "Rsc", Count: 4}
	tests := []struct {
		name       string
		top        string
		domains    []string
		stats      []ingress.CacheStats
		wantCodes  []string
		wantDoms   [][]string
		wantPreset string
	}{
		{"none", "", nil, nil, []string{}, nil, ""},
		{"next.js on one domain", "", []string{"a.test", "b.test"},
			[]ingress.CacheStats{{Reasons: []ingress.CacheReason{rsc}}, {}},
			[]string{"vary_header"}, [][]string{{"a.test"}}, "nextjs"},
		{"merged across domains", "", []string{"a.test", "b.test"},
			[]ingress.CacheStats{{Reasons: []ingress.CacheReason{{Code: "set_cookie", Count: 1}}}, {Reasons: []ingress.CacheReason{{Code: "set_cookie", Count: 2}}}},
			[]string{"set_cookie"}, [][]string{{"a.test", "b.test"}}, ""},
		{"top is env capped", "1", []string{"a.test"},
			[]ingress.CacheStats{{Reasons: []ingress.CacheReason{{Code: "private", Count: 5}, {Code: "no_store", Count: 2}}}},
			[]string{"private"}, [][]string{{"a.test"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.top != "" {
				t.Setenv(ingress.EnvCacheTopReasons, tt.top)
			}
			got := appReasons(tt.domains, tt.stats)
			codes := make([]string, 0, len(got))
			for i, r := range got {
				codes = append(codes, r.Code)
				if !slices.Equal(r.Domains, tt.wantDoms[i]) {
					t.Errorf("domains[%d] = %v, want %v", i, r.Domains, tt.wantDoms[i])
				}
			}
			if !slices.Equal(codes, tt.wantCodes) {
				t.Fatalf("codes = %v, want %v", codes, tt.wantCodes)
			}
			if p := suggestedPreset(got); p != tt.wantPreset {
				t.Errorf("suggestedPreset = %q, want %q", p, tt.wantPreset)
			}
		})
	}
}
