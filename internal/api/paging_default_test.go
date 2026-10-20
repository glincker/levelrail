package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageQueryDefault(t *testing.T) {
	items := make([]int, 250)
	tests := []struct {
		name, query string
		want        int
		wantTotal   string
	}{
		{"no limit uses default", "", 100, "250"},
		{"explicit limit wins", "?limit=10", 10, "250"},
		{"bad limit uses default", "?limit=x", 100, "250"},
		{"offset past default window", "?offset=200", 50, "250"},
		{"limit capped", "?limit=100000", 250, "250"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x"+tt.query, nil)
			got := pageQueryDefault(rec, req, items, 100)
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
			if h := rec.Header().Get("X-Total-Count"); h != tt.wantTotal {
				t.Fatalf("X-Total-Count = %q, want %q", h, tt.wantTotal)
			}
		})
	}
}
