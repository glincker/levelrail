package bitbucketapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

func TestListRepos_PastOldCapAndTruncation(t *testing.T) {
	const pages = 30
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var page int
		_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		page = max(page, 1)
		values := []map[string]any{{"full_name": fmt.Sprintf("ws/r%d", page), "name": "r"}}
		if page == 2 {
			values[0]["parent"] = map[string]any{"full_name": "up/stream"}
		}
		next := ""
		if page < pages {
			next = fmt.Sprintf("%s/repositories?role=member&pagelen=100&page=%d", srv.URL, page+1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"values": values, "next": next})
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}

	got, res, err := c.ListRepos(t.Context(), "t", gitprovider.ListOptions{})
	if err != nil || len(got) != pages || res.Truncated {
		t.Fatalf("len=%d truncated=%v err=%v, want %d untruncated", len(got), res.Truncated, err, pages)
	}
	if got[0].Fork || !got[1].Fork {
		t.Fatalf("fork flag not mapped: %+v %+v", got[0], got[1])
	}
	got, res, err = c.ListRepos(t.Context(), "t", gitprovider.ListOptions{MaxItems: 10})
	if err != nil || len(got) != 10 || !res.Truncated {
		t.Fatalf("len=%d truncated=%v err=%v, want 10 truncated", len(got), res.Truncated, err)
	}
}
