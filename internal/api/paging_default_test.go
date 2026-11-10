package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSortQueryAndPage(t *testing.T) {
	type row struct{ name string }
	cmps := map[string]func(a, b row) int{
		"name": func(a, b row) int { return strings.Compare(a.name, b.name) },
	}
	tests := []struct {
		name, query, want string
	}{
		{"ascending", "?sort=name", "a,b,c"},
		{"descending", "?sort=-name", "c,b,a"},
		{"unknown key keeps order", "?sort=bogus", "b,c,a"},
		{"sort then limit", "?sort=-name&limit=2", "c,b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []row{{"b"}, {"c"}, {"a"}}
			req := httptest.NewRequest(http.MethodGet, "/x"+tt.query, nil)
			sortQuery(req, items, cmps)
			var names []string
			for _, it := range pageQuery(httptest.NewRecorder(), req, items) {
				names = append(names, it.name)
			}
			if got := strings.Join(names, ","); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}
