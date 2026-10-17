package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

func testGitCheckout(ctx context.Context, repoURL, ref string) (string, string, func(), error) {
	rt := &Router{sourceCache: sourcecache.New(sourcecache.Config{}, nil)}
	return rt.gitCheckout(ctx, repoURL, ref, "")
}

const deadHostRepo = "http://127.0.0.1:1/org/web.git"

// newDeadHostRouter wires app "web" to a git host that refuses connections,
// with a real cache and no retries so the failure is immediate.
func newDeadHostRouter(t *testing.T) (*Router, gitSourceResource, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	rt.sourceCache = sourcecache.New(sourcecache.Config{Dir: t.TempDir(), Retries: 0, FetchTimeout: 5 * time.Second}, nil)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"`+deadHostRepo+`","branch":"main","build_type":"dockerfile"}`)
	rt.builder = &fakeBuilder{tag: "web:sha1"}
	return rt, created, cookie
}

func postDeadHostPush(t *testing.T, rt *Router, secret string) *httptest.ResponseRecorder {
	t.Helper()
	body := pushBody("refs/heads/main")
	return postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(secret), body), body, nil)
}

func TestWebhookWhileGitHostDown(t *testing.T) {
	rt, created, cookie := newDeadHostRouter(t)

	rec := postDeadHostPush(t, rt, created.WebhookSecret)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "git host unreachable, commit not cached") {
		t.Fatalf("status = %d body = %q, want 503 with the reason", rec.Code, rec.Body.String())
	}
	down := rt.sourceCache.Unreachable()
	if len(down) != 1 || len(down[0].Blocked) != 1 || down[0].Blocked[0] != "web" {
		t.Fatalf("unreachable = %+v, want one host blocking web", down)
	}

	feed := httptest.NewRecorder()
	rt.Handler().ServeHTTP(feed, authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", ""))
	if feed.Code != http.StatusOK || !strings.Contains(feed.Body.String(), `"git_host_unreachable"`) {
		t.Fatalf("attention feed = %d %s, want a git_host_unreachable item", feed.Code, feed.Body.String())
	}
}

func TestPushReplaysWhenGitHostRecovers(t *testing.T) {
	rt, created, _ := newDeadHostRouter(t)
	if rec := postDeadHostPush(t, rt, created.WebhookSecret); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb

	rt.replayBlockedPushes(context.Background(), "127.0.0.1", []string{"web"})
	if fb.calls != 1 || len(fetchCalls) != 1 || fetchCalls[0].sha != "sha1" {
		t.Fatalf("builder calls = %d fetch calls = %+v, want one replayed deploy of sha1", fb.calls, fetchCalls)
	}
}

func TestReplaySkipsWhenNewestDeliveryIsNotBlocked(t *testing.T) {
	rt, created, _ := newDeadHostRouter(t)
	postDeadHostPush(t, rt, created.WebhookSecret)

	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb
	rt.saveWebhookDelivery(context.Background(), "web", "github", "push", nil, true, true, http.StatusOK, pushBody("refs/heads/main"), "", "deployed")

	if err := rt.replayNewestBlockedDelivery(context.Background(), "web", time.Hour); err != nil {
		t.Fatal(err)
	}
	if fb.calls != 0 {
		t.Fatalf("builder calls = %d, want 0: a newer delivery supersedes the blocked one", fb.calls)
	}
}

func TestDeployFromCache(t *testing.T) {
	fb := newFakeBuilder("local/web:built", nil)
	rt, db := newTestRouterWithBuilder(t, fb, nil)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)
	seedGitSourceForSchedule(t, db)

	offline := make(chan bool, 1)
	refs := make(chan string, 1)
	rt.fetch = func(ctx context.Context, _, ref, _ string) (string, string, func(), error) {
		offline <- sourcecache.IsOffline(ctx)
		refs <- ref
		return t.TempDir(), ref, func() {}, nil
	}

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys/from-cache", body))
		return rec
	}

	if rec := post(`{}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no earlier deploy: status = %d, want 422; body = %s", rec.Code, rec.Body.String())
	}

	if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: "dep_good", ServiceName: "web", Image: "levelrail/web:abc", CommitSHA: "abc1234",
		Source: store.DeployAttemptSourceWebhook, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if rec := post(`{}`); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", rec.Code, rec.Body.String())
	}
	if !<-offline || <-refs != "abc1234" {
		t.Fatal("want an offline fetch of the last good commit")
	}
	fb.awaitCall(t)
}
