package githubapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

func pagedRepos(total int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var page int
		_, _ = fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		start := (page - 1) * listPerPage
		end := min(start+listPerPage, total)
		var repos []repoResponse
		for i := start; i < end; i++ {
			repos = append(repos, repoResponse{FullName: fmt.Sprintf("acme/repo-%05d", i), Fork: i%2 == 0, Archived: i%3 == 0})
		}
		w.Header().Set("ETag", fmt.Sprintf(`"page-%d"`, page))
		_ = json.NewEncoder(w).Encode(listRepositoriesResponse{TotalCount: total, Repositories: repos})
	}
}

func TestListInstallationRepos_PastOldCap(t *testing.T) {
	// 4,550 repos is 46 pages: beyond the old 20 page (2,000 repo) cut-off.
	const total = 4550
	c := newTestClient(t, pagedRepos(total))
	repos, res, err := c.ListInstallationRepos(t.Context(), "", "tok", ListOptions{})
	if err != nil {
		t.Fatalf("ListInstallationRepos() error = %v", err)
	}
	if len(repos) != total || res.Truncated {
		t.Fatalf("got %d repos truncated=%v, want %d untruncated", len(repos), res.Truncated, total)
	}
	if !repos[0].Fork || !repos[0].Archived || repos[1].Fork {
		t.Fatalf("fork/archived flags not mapped: %+v %+v", repos[0], repos[1])
	}
}

func TestListInstallationRepos_CapMarksTruncated(t *testing.T) {
	c := newTestClient(t, pagedRepos(950))
	repos, res, err := c.ListInstallationRepos(t.Context(), "", "tok", ListOptions{ListOptions: gitprovider.ListOptions{MaxItems: 300}})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(repos) != 300 || !res.Truncated {
		t.Fatalf("got %d repos truncated=%v, want 300 truncated", len(repos), res.Truncated)
	}
}

func TestListInstallationRepos_ConditionalPagesAreReused(t *testing.T) {
	var full, notModified atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		full.Add(1)
		pagedRepos(250)(w, r)
	})
	first, res, err := c.ListInstallationRepos(t.Context(), "", "tok", ListOptions{})
	if err != nil || len(first) != 250 || len(res.ETags) != 3 {
		t.Fatalf("first listing: %d repos, %d etags, err %v", len(first), len(res.ETags), err)
	}
	second, res2, err := c.ListInstallationRepos(t.Context(), "", "tok", ListOptions{PriorETags: res.ETags, PriorRepos: first})
	if err != nil {
		t.Fatalf("second listing error = %v", err)
	}
	if len(second) != 250 || res2.NotModified != 3 || notModified.Load() != 3 || full.Load() != 3 {
		t.Fatalf("second listing: %d repos, notModified=%d (server saw %d), full responses=%d", len(second), res2.NotModified, notModified.Load(), full.Load())
	}
}

func TestListInstallationRepos_RateLimitRetriesThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		pagedRepos(5)(w, r)
	})
	var slept time.Duration
	opts := ListOptions{ListOptions: gitprovider.ListOptions{Retry: gitprovider.RetryPolicy{
		Retries: 2, MaxWait: 5 * time.Second,
		Sleep: func(_ context.Context, d time.Duration) error { slept += d; return nil },
	}}}
	repos, _, err := c.ListInstallationRepos(t.Context(), "", "tok", opts)
	if err != nil || len(repos) != 5 || slept != time.Second {
		t.Fatalf("repos=%d slept=%v err=%v, want 5 repos after one 1s wait", len(repos), slept, err)
	}
}

func TestListInstallationRepos_LongRateLimitIsTyped(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit"}`))
	})
	opts := ListOptions{ListOptions: gitprovider.ListOptions{Retry: gitprovider.RetryPolicy{Retries: 2, MaxWait: time.Second}}}
	_, _, err := c.ListInstallationRepos(t.Context(), "", "tok", opts)
	rl, ok := gitprovider.AsRateLimit(err)
	if !ok || rl.RetryAfter != 10*time.Minute {
		t.Fatalf("err = %v, want a RateLimitError with a 10m retry-after", err)
	}
}
