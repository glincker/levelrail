package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/branchdeploy"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

func TestStackDeployHandlers_MemberDenyIsUnknownName(t *testing.T) {
	rt, db := newTestRouter(t)
	seedStackAttempt(t, db, "shop")
	denied := seedMatrixToken(t, db, "member-deny", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-db", "Deny", "*", "app:shop-db", store.PrincipalTypeToken, "member-deny")
	plain := seedMatrixToken(t, db, "plain", []string{AbilityRead})

	get := func(tok, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		bearer(tok)(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	notes := "/api/v1/apps/shop/deploys/release-notes?from=dep_shop&to=dep_shop"
	if rec := get(denied, notes); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error":"app not found"`) {
		t.Fatalf("release notes for a denied member = %d %s, want the unknown-name 404", rec.Code, rec.Body.String())
	}
	if rec := get(plain, notes); strings.Contains(rec.Body.String(), "app not found") {
		t.Fatalf("release notes for a readable stack = %d %s, want no app-not-found", rec.Code, rec.Body.String())
	}
}

func TestSearch_TeamOwnerFindsOwnStack(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	seedStackAttempt(t, f.db, "shopb")
	for _, m := range []string{"shopb-web", "shopb-db"} {
		if err := f.db.UpdateServiceProject(ctx, m, projB); err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetServiceEnvironment(ctx, m, envB); err != nil {
			t.Fatal(err)
		}
	}
	path := "/api/v1/search?q=" + url.QueryEscape("kind:deployment shopb")
	code, body := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(f.bOwner))
	if code != http.StatusOK || !strings.Contains(body, "dep_shopb") {
		t.Fatalf("org B owner search = %d %s, want its own stack deployment", code, body)
	}
	code, body = teamDo(t, f.rt, http.MethodGet, path, "", withCookie(f.cookies[store.TeamRoleOwner]))
	if code != http.StatusOK || strings.Contains(body, "dep_shopb") {
		t.Fatalf("org A search = %d %s, want 200 without the stack deployment", code, body)
	}
}

func TestPreviewNames_RefuseStackName(t *testing.T) {
	rt, db, _, _ := setUpPreviewApp(t)
	ctx := context.Background()
	slug := branchSlug("web", "feature-x")
	seedStackAttempt(t, db, branchdeploy.AppName("web", slug))
	if row, status, msg := rt.beginBranchDeploy(ctx, "web", "feature-x", slug, webhook.PushEvent{After: "abc123"}); row != nil || status != http.StatusConflict {
		t.Fatalf("branch deploy onto a stack name = %d %q, want 409 and no row", status, msg)
	}
	seedStackAttempt(t, db, previewAppName("web", 7))
	ev := webhook.PullRequestEvent{Number: 7, HeadRef: "feature-y", HeadSHA: "def456"}
	if row, done, status, msg := rt.beginPreview(ctx, "web", store.GitSource{}, ev, false); row != nil || !done || status != http.StatusConflict {
		t.Fatalf("preview onto a stack name = %d %q, want 409 and no row", status, msg)
	}
}

func TestAppNameFree_UnderLock(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedStackAttempt(t, db, "shop")
	if err := rt.appNameFree(ctx, "shop"); !errors.Is(err, errAppNameTaken) {
		t.Fatalf("stack name: err = %v, want taken", err)
	}
	if err := rt.appNameFree(ctx, "shop-web"); !errors.Is(err, errAppNameTaken) {
		t.Fatalf("service name: err = %v, want taken", err)
	}
	if err := rt.appNameFree(ctx, "free-name"); err != nil {
		t.Fatalf("free name: err = %v", err)
	}
	if _, err := rt.cloneOneApp(ctx, store.DesiredService{Name: "src", Image: "nginx:1", Port: 80}, "shop", "", nil, false); !errors.Is(err, errAppNameTaken) {
		t.Fatalf("environment clone onto a stack name: err = %v, want taken", err)
	}
	if err := rt.saveClonedService(ctx, store.DesiredService{Name: "shop", Image: "nginx:1", Port: 80}); !errors.Is(err, errAppNameTaken) {
		t.Fatalf("project clone onto a stack name: err = %v, want taken", err)
	}
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"shop","image":"nginx:1","port":80}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("create onto a stack name = %d %s, want 409", rec.Code, rec.Body.String())
	}
}

// seedServiceTookStackName makes "shop" a service that shares its name with a stack.
func seedServiceTookStackName(t *testing.T, db *store.DB) {
	t.Helper()
	seedStackAttempt(t, db, "shop")
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "shop", AppID: "shop", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
}

var stackGuardRequestID = regexp.MustCompile(`"request_id":"[^"]*"`)

// bodyOf drops the per-request id, the only byte that may differ between two 404s.
func bodyOf(rec *httptest.ResponseRecorder) string {
	return stackGuardRequestID.ReplaceAllString(rec.Body.String(), "")
}

func serveAs(rt *Router, tok, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestStackDeployHandlers_DeniedMatchesUnknownByteForByte(t *testing.T) {
	rt, db := newTestRouter(t)
	seedServiceTookStackName(t, db)
	tok := seedMatrixToken(t, db, "member-deny", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-db", "Deny", "*", "app:shop-db", store.PrincipalTypeToken, "member-deny")
	for _, suffix := range []string{"/deploys/dep_shop", "/timeline", "/deploys", "/deploys/dep_shop/probes", "/deploys/release-notes?from=dep_shop"} {
		denied := serveAs(rt, tok, http.MethodGet, "/api/v1/apps/shop"+suffix)
		unknown := serveAs(rt, tok, http.MethodGet, "/api/v1/apps/zz-missing"+suffix)
		if denied.Code != http.StatusNotFound || denied.Code != unknown.Code || bodyOf(denied) != bodyOf(unknown) {
			t.Fatalf("%s: denied = %d %q, unknown = %d %q", suffix, denied.Code, denied.Body.String(), unknown.Code, unknown.Body.String())
		}
	}
}

func TestDeployOnlyToken_CancelAndRetry(t *testing.T) {
	rt, db, _ := newTimelineRouter(t)
	ctx := context.Background()
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: "https://example.com/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	rt.buildRetrier = func(context.Context, string, string) (string, error) { return "queued", nil }
	saveAttempt(t, db, "att_queued", store.DeployAttemptStatusQueued)
	saveAttempt(t, db, "att_failed", store.DeployAttemptStatusFailed)
	seedServiceTookStackName(t, db)
	tok := seedMatrixToken(t, db, "deployer", []string{AbilityDeploy})
	attachTestPolicy(t, db, "deny-db", "Deny", "*", "app:shop-db", store.PrincipalTypeToken, "deployer")

	if rec := serveAs(rt, tok, http.MethodPost, "/api/v1/apps/web/deploys/att_queued/cancel"); rec.Code != http.StatusOK {
		t.Fatalf("deploy-only cancel = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if rec := serveAs(rt, tok, http.MethodPost, "/api/v1/apps/web/builds/att_failed/retry"); rec.Code != http.StatusAccepted {
		t.Fatalf("deploy-only retry = %d %s, want 202", rec.Code, rec.Body.String())
	}
	denied := serveAs(rt, tok, http.MethodPost, "/api/v1/apps/shop/deploys/dep_shop/cancel")
	unknown := serveAs(rt, tok, http.MethodPost, "/api/v1/apps/zz-missing/deploys/dep_shop/cancel")
	if denied.Code != http.StatusNotFound || bodyOf(denied) != bodyOf(unknown) {
		t.Fatalf("cancel with an unreachable member = %d %q, unknown = %d %q", denied.Code, denied.Body.String(), unknown.Code, unknown.Body.String())
	}
}

func TestImportBundleAndComposeDeploy_SameNameNoDeadlock(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	body := bundleBody(t)
	compose := []byte("services:\n  web:\n    image: nginx:1\n")
	var wg sync.WaitGroup
	codes := make(chan int, 4)
	for i := 0; i < 2; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/import-bundle?name=shop&replace=true", body))
			codes <- rec.Code
		}()
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/shop/compose", nil)
			rt.lockedComposeDeploy(httptest.NewRecorder(), req, "shop", compose, composeDeployOpts{noCaller: true})
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("bundle import and compose deploy for one name did not finish: lock order")
	}
	close(codes)
	for c := range codes {
		if c >= http.StatusInternalServerError {
			t.Fatalf("bundle import = %d, want no server error", c)
		}
	}
}
