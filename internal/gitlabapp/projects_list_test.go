package gitlabapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

func TestListProjects_PastOldCapAndTruncation(t *testing.T) {
	const total = 2750
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var page int
		_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		start := (page - 1) * listPerPage
		end := min(start+listPerPage, total)
		var out []map[string]any
		for i := start; i < end; i++ {
			p := map[string]any{"id": i + 1, "name": "p", "path_with_namespace": fmt.Sprintf("g/p%d", i), "archived": i == 0}
			if i == 1 {
				p["forked_from_project"] = map[string]any{"id": 9}
			}
			out = append(out, p)
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}

	got, res, err := c.ListProjects(t.Context(), srv.URL, "at", gitprovider.ListOptions{})
	if err != nil || len(got) != total || res.Truncated {
		t.Fatalf("len=%d truncated=%v err=%v, want %d untruncated", len(got), res.Truncated, err, total)
	}
	if !got[0].Archived || !got[1].Fork || got[2].Fork {
		t.Fatalf("archived/fork flags not mapped: %+v %+v", got[0], got[1])
	}

	got, res, err = c.ListProjects(t.Context(), srv.URL, "at", gitprovider.ListOptions{MaxItems: 500})
	if err != nil || len(got) != 500 || !res.Truncated {
		t.Fatalf("len=%d truncated=%v err=%v, want 500 truncated", len(got), res.Truncated, err)
	}
}
