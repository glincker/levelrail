package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func hookTestSite(t *testing.T, withGit bool) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	cookie := loginTestSession(t, rt, db)
	seedStaticSite(t, db, t.TempDir(), "docs", "d.example.com")
	seedApp(t, db, "docs")
	if withGit {
		gs := store.GitSource{ServiceName: "docs", RepoURL: "https://git.invalid/acme/docs.git", Branch: "main", BuildType: "static"}
		if err := db.SaveGitSource(context.Background(), gs); err != nil {
			t.Fatal(err)
		}
	}
	return rt, db, cookie
}

func mintSiteHook(t *testing.T, rt *Router, cookie *http.Cookie) (staticSiteHookResource, string) {
	t.Helper()
	body := ""
	if hash, _, _ := rt.staticSites.(staticSiteHookStore).GetStaticSiteDeployHook(context.Background(), "docs"); hash != "" {
		body = `{"rotate":true}`
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites/docs/hook", body))
	var res staticSiteHookResource
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("mint = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !strings.Contains(res.URL, pagesHookURLPrefix+"docs/") {
		t.Fatalf("mint body = %s, %v", rec.Body.String(), err)
	}
	return res, res.URL[strings.Index(res.URL, pagesHookURLPrefix):]
}

func TestPagesHookRefusedWithoutGitSource(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites/docs/hook", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("no git source = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestPagesHookSecretStoredHashedAndShownOnce(t *testing.T) {
	rt, db, cookie := hookTestSite(t, true)
	res, path := mintSiteHook(t, rt, cookie)
	secret := path[strings.LastIndex(path, "/")+1:]
	hash, _, err := db.GetStaticSiteDeployHook(context.Background(), "docs")
	if err != nil || hash == "" || hash == secret || len(secret) < 43 {
		t.Fatalf("stored = %q (secret len %d), %v", hash, len(secret), err)
	}
	if !res.Enabled {
		t.Fatalf("create body not enabled: %+v", res)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/docs/hook", ""))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), hash) || strings.Contains(rec.Body.String(), "url") {
		t.Fatalf("get = %d leaked: %s", rec.Code, rec.Body.String())
	}
}

func TestPagesHookTriggerAuth(t *testing.T) {
	rt, _, cookie := hookTestSite(t, true)
	_, path := mintSiteHook(t, rt, cookie)
	if got := postHook(rt, path[:strings.LastIndex(path, "/")+1]+"wrong", "").Code; got != http.StatusNotFound {
		t.Errorf("wrong secret = %d, want 404", got)
	}
	if got := postHook(rt, pagesHookURLPrefix+"nosuch/"+path[strings.LastIndex(path, "/")+1:], "").Code; got != http.StatusNotFound {
		t.Errorf("unknown site = %d, want 404", got)
	}
	if got := postHook(rt, path, "").Code; got != http.StatusAccepted {
		t.Errorf("right secret = %d, want 202", got)
	}
}

func TestPagesHookRotateInvalidatesOldAndRevokeStops(t *testing.T) {
	rt, _, cookie := hookTestSite(t, true)
	_, oldPath := mintSiteHook(t, rt, cookie)
	_, newPath := mintSiteHook(t, rt, cookie)
	if oldPath == newPath {
		t.Fatal("rotate returned the same secret")
	}
	if got := postHook(rt, oldPath, "").Code; got != http.StatusNotFound {
		t.Errorf("old secret after rotate = %d, want 404", got)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/static-sites/docs/hook", "")); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", rec.Code)
	}
	if got := postHook(rt, newPath, "").Code; got != http.StatusNotFound {
		t.Errorf("revoked secret = %d, want 404", got)
	}
}

func TestPagesHookRunningDeployAnswers409(t *testing.T) {
	rt, db, cookie := hookTestSite(t, true)
	_, path := mintSiteHook(t, rt, cookie)
	err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: "dep_run1", ServiceName: "docs", Image: "i", Source: "manual",
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := postHook(rt, path, "").Code; got != http.StatusConflict {
		t.Fatalf("running deploy = %d, want 409", got)
	}
}

func TestPagesHookRateLimited(t *testing.T) {
	t.Setenv(envBuildHookRate, "2")
	rt, _, cookie := hookTestSite(t, true)
	_, path := mintSiteHook(t, rt, cookie)
	var codes []int
	for i := 0; i < 4; i++ {
		codes = append(codes, postHook(rt, path, "").Code)
	}
	if codes[len(codes)-1] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v, want the last to be 429", codes)
	}
}

func TestPagesHookCreateAnswers409WhenOneExistsAndRotateIsExplicit(t *testing.T) {
	rt, _, cookie := hookTestSite(t, true)
	mintSiteHook(t, rt, cookie)
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites/docs/hook", "")); rec.Code != http.StatusConflict {
		t.Fatalf("second create = %d, want 409", rec.Code)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/static-sites/docs/hook", "")); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", rec.Code)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites/docs/hook", `{"rotate":true}`)); rec.Code != http.StatusConflict {
		t.Fatalf("rotate without hook = %d, want 409", rec.Code)
	}
}

func TestPagesHookClearedWhenGitSourceDisconnected(t *testing.T) {
	rt, db, cookie := hookTestSite(t, true)
	_, path := mintSiteHook(t, rt, cookie)
	if rec := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/docs/git-source", "")); rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect = %d: %s", rec.Code, rec.Body.String())
	}
	if hash, _, _ := db.GetStaticSiteDeployHook(context.Background(), "docs"); hash != "" {
		t.Fatal("hook hash survived the git source")
	}
	gs := store.GitSource{ServiceName: "docs", RepoURL: "https://git.invalid/acme/docs.git", Branch: "main", BuildType: "static"}
	if err := db.SaveGitSource(context.Background(), gs); err != nil {
		t.Fatal(err)
	}
	if got := postHook(rt, path, "").Code; got != http.StatusNotFound {
		t.Fatalf("old hook after reconnect = %d, want 404", got)
	}
}

func TestPagesHookRefusedWhenMintedBeforeCurrentGitSource(t *testing.T) {
	rt, db, cookie := hookTestSite(t, true)
	_, path := mintSiteHook(t, rt, cookie)
	later := store.GitSource{ServiceName: "docs", RepoURL: "https://git.invalid/acme/docs.git", Branch: "main", BuildType: "static"}
	if err := db.DeleteGitSource(context.Background(), "docs"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := db.SaveGitSource(context.Background(), later); err != nil {
		t.Fatal(err)
	}
	if got := postHook(rt, path, "").Code; got != http.StatusNotFound {
		t.Fatalf("hook of an earlier connection = %d, want 404", got)
	}
}

func TestPagesHookTriggerAuditCarriesHookCreatedAt(t *testing.T) {
	rt, db, cookie := hookTestSite(t, true)
	res, path := mintSiteHook(t, rt, cookie)
	if got := postHook(rt, path, "").Code; got != http.StatusAccepted {
		t.Fatalf("trigger = %d", got)
	}
	entries, err := db.ListAuditEntries(context.Background(), 20, nil, store.AuditEntryFilter{Action: AuditActionStaticSiteHookTriggered})
	if err != nil || len(entries) == 0 {
		t.Fatalf("audit = %v, %v", entries, err)
	}
	if !strings.Contains(entries[0].Detail, res.CreatedAt.UTC().Format(time.RFC3339Nano)) || strings.Contains(entries[0].Path, path[strings.LastIndex(path, "/")+1:]) {
		t.Fatalf("detail %q path %q", entries[0].Detail, entries[0].Path)
	}
}
