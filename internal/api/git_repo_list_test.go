package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

func getRepoList(t *testing.T, rt *Router, cookie *http.Cookie, query string) (int, http.Header, gitRepoListResource, gitRepoRateLimitedError) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/git/repos?"+query, ""))
	var ok gitRepoListResource
	var limited gitRepoRateLimitedError
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil {
			t.Fatalf("decode: %v: %s", err, rec.Body.String())
		}
	} else {
		_ = json.Unmarshal(rec.Body.Bytes(), &limited)
	}
	return rec.Code, rec.Header(), ok, limited
}

func manyGitLabProjects(n int) []gitlabapp.Project {
	out := make([]gitlabapp.Project, n)
	for i := range out {
		out[i] = gitlabapp.Project{
			ID: int64(i + 1), Name: fmt.Sprintf("p%d", i), PathWithNamespace: fmt.Sprintf("grp%d/p%d", i%3, i),
			HTTPURLToRepo: fmt.Sprintf("https://gl.example.com/grp%d/p%d.git", i%3, i), DefaultBranch: "main",
			Visibility: "private", Archived: i%10 == 0, Fork: i%7 == 0,
		}
	}
	return out
}

func TestGitRepoList_GitLabSearchFilterPageAndCache(t *testing.T) {
	fake := &fakeGitLabAppClient{projects: manyGitLabProjects(2300)}
	rt, db := newTestRouterWithGitLabApp(t, authorizedGitLabSecrets(t), fake)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	code, _, first, _ := getRepoList(t, rt, cookie, "provider=gitlab&limit=100")
	if code != http.StatusOK || first.Total != 2300 || len(first.Items) != 100 || !first.HasMore || first.NextOffset == nil || *first.NextOffset != 100 {
		t.Fatalf("first page: code=%d total=%d items=%d next=%v", code, first.Total, len(first.Items), first.NextOffset)
	}
	if first.Truncated || first.Loaded != 2300 {
		t.Fatalf("a list past the old 2000 cap must load whole: loaded=%d truncated=%v", first.Loaded, first.Truncated)
	}

	_, _, searched, _ := getRepoList(t, rt, cookie, "provider=gitlab&search=p2299")
	if searched.Total != 1 || searched.Items[0].FullName != "grp1/p2299" {
		t.Fatalf("search past the first 2000: %+v", searched.Items)
	}

	_, _, noArchived, _ := getRepoList(t, rt, cookie, "provider=gitlab&archived=exclude&forks=exclude")
	if noArchived.Total >= 2300 || noArchived.Total != countMatching(fake.projects, func(p gitlabapp.Project) bool { return !p.Archived && !p.Fork }) {
		t.Fatalf("archived+forks exclusion total = %d", noArchived.Total)
	}
	_, _, owner, _ := getRepoList(t, rt, cookie, "provider=gitlab&owner=grp1")
	if owner.Total == 0 || owner.Items[0].Owner != "grp1" || len(owner.Owners) != 3 {
		t.Fatalf("owner filter/facets: total=%d owners=%+v", owner.Total, owner.Owners)
	}

	if got := fake.listCalls.Load(); got != 1 {
		t.Fatalf("provider listings = %d, want 1 for five picker requests", got)
	}
	getRepoList(t, rt, cookie, "provider=gitlab&refresh=1")
	if got := fake.listCalls.Load(); got != 1 {
		t.Fatalf("a refresh inside RefreshMin must not hit the provider again, listings = %d", got)
	}
}

func countMatching(ps []gitlabapp.Project, keep func(gitlabapp.Project) bool) int {
	n := 0
	for _, p := range ps {
		if keep(p) {
			n++
		}
	}
	return n
}

func TestGitRepoList_ManualRefreshAndInvalidationOnDisconnect(t *testing.T) {
	fake := &fakeGitLabAppClient{projects: manyGitLabProjects(5)}
	rt, db := newTestRouterWithGitLabApp(t, authorizedGitLabSecrets(t), fake)
	rt.repoCache = newRepoCacheForTest()
	cookie := loginTestSession(t, rt, db)
	conn := seedGitLabAppConnection(t, db)

	getRepoList(t, rt, cookie, "provider=gitlab")
	getRepoList(t, rt, cookie, "provider=gitlab")
	if fake.listCalls.Load() != 1 {
		t.Fatalf("listings = %d, want 1", fake.listCalls.Load())
	}
	getRepoList(t, rt, cookie, "provider=gitlab&refresh=1")
	if fake.listCalls.Load() != 2 {
		t.Fatalf("a manual refresh must re-list, listings = %d", fake.listCalls.Load())
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/git-connections/"+conn.ID, ""))
	if rec.Code >= 300 {
		t.Fatalf("disconnect status = %d: %s", rec.Code, rec.Body.String())
	}
	if code, _, _, _ := getRepoList(t, rt, cookie, "provider=gitlab"); code != http.StatusConflict {
		t.Fatalf("after disconnect the cached list must be gone, status = %d", code)
	}
}

func TestGitRepoList_RateLimitedIsTyped429WithRetryAfter(t *testing.T) {
	next := time.Now().Add(90 * time.Second)
	fake := &fakeGitLabAppClient{projectsErr: &gitprovider.RateLimitError{Provider: "gitlab", NextCheck: next, RetryAfter: 90 * time.Second}}
	rt, db := newTestRouterWithGitLabApp(t, authorizedGitLabSecrets(t), fake)
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)

	code, hdr, _, body := getRepoList(t, rt, cookie, "provider=gitlab")
	if code != http.StatusTooManyRequests || body.Code != codeRateLimited || body.NextCheckAt.IsZero() || hdr.Get("Retry-After") == "" {
		t.Fatalf("code=%d retry-after=%q body=%+v, want a typed 429 with a next check time", code, hdr.Get("Retry-After"), body)
	}
	if body.RetryAfterSeconds < 85 || body.RetryAfterSeconds > 95 {
		t.Fatalf("retry_after_seconds = %d, want about 90", body.RetryAfterSeconds)
	}
}

func TestGitRepoList_RateLimitedRefreshServesStaleList(t *testing.T) {
	fake := &fakeGitLabAppClient{projects: manyGitLabProjects(3)}
	rt, db := newTestRouterWithGitLabApp(t, authorizedGitLabSecrets(t), fake)
	rt.repoCache = newRepoCacheForTest()
	cookie := loginTestSession(t, rt, db)
	seedGitLabAppConnection(t, db)
	getRepoList(t, rt, cookie, "provider=gitlab")

	fake.projectsErr = &gitprovider.RateLimitError{Provider: "gitlab", NextCheck: time.Now().Add(time.Minute)}
	code, _, res, _ := getRepoList(t, rt, cookie, "provider=gitlab&refresh=1")
	if code != http.StatusOK || !res.Stale || res.RateLimit == nil || res.Total != 3 {
		t.Fatalf("code=%d stale=%v rate_limit=%v total=%d, want the cached list with a rate limit notice", code, res.Stale, res.RateLimit, res.Total)
	}
}

func TestGitRepoList_GitHubSameNameDifferentOwnersAndTruncation(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	fake := &fakeGitHubAppClient{
		mintToken: githubapp.InstallationToken{Token: "ghs_x"},
		repos: []githubapp.Repo{
			{FullName: "acme/api", Name: "api", OwnerLogin: "acme", DefaultBranch: "main"},
			{FullName: "octocat/api", Name: "api", OwnerLogin: "octocat", DefaultBranch: "main", Fork: true},
		},
		listResult: githubapp.ListResult{Truncated: true},
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fake)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	_, _, res, _ := getRepoList(t, rt, cookie, "provider=github&search=api")
	if res.Total != 2 || res.Items[0].Owner != "acme" || res.Items[1].Owner != "octocat" || !res.Truncated {
		t.Fatalf("result = %+v, want both owners' api repos and the truncated flag", res)
	}
	_, _, forks, _ := getRepoList(t, rt, cookie, "provider=github&forks=exclude")
	if forks.Total != 1 || forks.Items[0].Owner != "acme" {
		t.Fatalf("forks excluded = %+v", forks.Items)
	}
	if fake.gotListOpts.MaxItems != rt.repoCache.Config().MaxItems {
		t.Fatalf("provider cap = %d, want the cache cap %d", fake.gotListOpts.MaxItems, rt.repoCache.Config().MaxItems)
	}
}

func TestGitRepoList_RejectsUnknownProvider(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if code, _, _, _ := getRepoList(t, rt, cookie, "provider=svn"); code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
}
