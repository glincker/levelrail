package giteaapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

func TestListRepos_PastOldCapAndTruncation(t *testing.T) {
	const total = 1275
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var page int
		_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		start := (page - 1) * listPerPage
		end := min(start+listPerPage, total)
		var out []map[string]any
		for i := start; i < end; i++ {
			out = append(out, map[string]any{"full_name": fmt.Sprintf("o/r%d", i), "archived": i == 0, "fork": i == 1})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}

	got, res, err := c.ListRepos(t.Context(), srv.URL, "t", gitprovider.ListOptions{})
	if err != nil || len(got) != total || res.Truncated {
		t.Fatalf("len=%d truncated=%v err=%v, want %d untruncated", len(got), res.Truncated, err, total)
	}
	if !got[0].Archived || !got[1].Fork {
		t.Fatalf("flags not mapped: %+v %+v", got[0], got[1])
	}
	got, res, err = c.ListRepos(t.Context(), srv.URL, "t", gitprovider.ListOptions{MaxItems: 400})
	if err != nil || len(got) != 400 || !res.Truncated {
		t.Fatalf("len=%d truncated=%v err=%v, want 400 truncated", len(got), res.Truncated, err)
	}
}
