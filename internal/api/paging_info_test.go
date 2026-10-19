package api

import (
	"net/http/httptest"
	"testing"
)

func TestPageWithInfo(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	tests := []struct {
		name      string
		query     string
		want      []int
		wantInfo  bool
		wantNext  int
		wantLimit int
	}{
		{name: "no params leaves list and envelope alone", query: "", want: items},
		{name: "first page", query: "?limit=2", want: []int{1, 2}, wantInfo: true, wantNext: 2, wantLimit: 2},
		{name: "last page has no next", query: "?limit=2&offset=4", want: []int{5}, wantInfo: true, wantNext: -1, wantLimit: 1},
		{name: "offset past end is empty", query: "?offset=99", want: []int{}, wantInfo: true, wantNext: -1, wantLimit: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/x"+tc.query, nil)
			got, info := pageWithInfo(rec, req, items)
			if len(got) != len(tc.want) {
				t.Fatalf("items = %v, want %v", got, tc.want)
			}
			if (info != nil) != tc.wantInfo {
				t.Fatalf("info = %+v, want present=%v", info, tc.wantInfo)
			}
			if rec.Header().Get("X-Total-Count") != "5" {
				t.Errorf("X-Total-Count = %q", rec.Header().Get("X-Total-Count"))
			}
			if info == nil {
				return
			}
			if info.Total != 5 || info.Limit != tc.wantLimit {
				t.Errorf("info = %+v", info)
			}
			if tc.wantNext < 0 && info.NextOffset != nil {
				t.Errorf("next_offset = %d, want none", *info.NextOffset)
			}
			if tc.wantNext >= 0 && (info.NextOffset == nil || *info.NextOffset != tc.wantNext) {
				t.Errorf("next_offset = %v, want %d", info.NextOffset, tc.wantNext)
			}
		})
	}
}
