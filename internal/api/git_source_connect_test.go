package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestGitSourcePlan_ImageAppKeepsConfigAndSuggestsBuild(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "nginx:1.27", Port: 80, Domains: []string{"web.example.test"},
		Env:     map[string]string{"A": "1"},
		Volumes: []store.ServiceVolume{{Name: "web-data", ContainerPath: "/data"}},
	}); err != nil {
		t.Fatal(err)
	}
	useFakeLayout(rt, &fakeLayoutSource{files: map[string]string{"Dockerfile": "FROM scratch\n"}})

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/plan", `{"repo_url":"https://github.com/org/web.git"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got gitSourcePlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CurrentSource != gitConnectSourceImage || got.Preserved.RunningImage != "nginx:1.27" || got.Preserved.EnvVars != 1 {
		t.Fatalf("current or preserved wrong: %+v", got)
	}
	if len(got.Preserved.Volumes) != 1 || len(got.Preserved.Domains) != 1 {
		t.Fatalf("volumes or domains not listed: %+v", got.Preserved)
	}
	if got.Suggestion == nil || got.Suggestion.BuildType == "" || got.Branch == "" {
		t.Fatalf("expected a build suggestion and default branch: %+v", got)
	}
	if rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/missing/git-source/plan", `{}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing app status %d", rec.Code)
	}
	if rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/plan", `{"repo_url":"file:///etc"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("file url status %d", rec.Code)
	}
}

func TestGitSourceDeploy_RequiresConnectedSource(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/deploy", `{}`)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusNotImplemented {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func waitGitDeployReleased(t *testing.T, rt *Router, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		rt.buildStartMu.Lock()
		n := rt.startingDeploys[name]
		rt.buildStartMu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background deploy never released its start mark")
}

func TestGitSourceDeploy_FreezeAndConcurrency(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, nil)
	rt.builder = &fakeBuilder{tag: "web:sha"}
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "http://127.0.0.1:1/org/web.git", Branch: "main", BuildType: "railpack"}); err != nil {
		t.Fatal(err)
	}
	if put := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/deploy-freeze", alwaysFrozen())); put.Code != http.StatusOK {
		t.Fatalf("freeze put = %d %s", put.Code, put.Body.String())
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/deploy", `{}`)); rec.Code != http.StatusLocked {
		t.Fatalf("frozen deploy status = %d %s", rec.Code, rec.Body.String())
	}

	rt.buildStartMu.Lock()
	rt.startingDeploys["web"]++
	rt.buildStartMu.Unlock()
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/deploy", `{"override_freeze":true,"override_reason":"hotfix"}`)); rec.Code != http.StatusConflict {
		t.Fatalf("busy deploy status = %d %s", rec.Code, rec.Body.String())
	}
	rt.buildStartMu.Lock()
	delete(rt.startingDeploys, "web")
	rt.buildStartMu.Unlock()

	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/deploy", `{"override_freeze":true,"override_reason":"hotfix"}`)); rec.Code != http.StatusAccepted {
		t.Fatalf("override deploy status = %d %s", rec.Code, rec.Body.String())
	}
	waitGitDeployReleased(t, rt, "web")
}

func TestGitSourcePlan_SSHURLIsNotDetectable(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/plan", `{"repo_url":"git@github.com:org/web.git"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"detect_error":"ssh_not_detectable"`) {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPlanRepoLayoutSource_NoCloneFallback(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:8080/org/web.git", "http://169.254.169.254/latest.git", "https://git.example.test/org/web.git"} {
		if _, err := planRepoLayoutSource(context.Background(), u, "main", "", repolayout.Hosts{}); err == nil {
			t.Errorf("%s: want an error, a clone fallback must not run", u)
		} else if planDetectError(err) != "unsupported_host" {
			t.Errorf("%s: code %q", u, planDetectError(err))
		}
	}
}

func TestGitSourcePlan_DeployOnlyTokenNeverMintsAppToken(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newPrivateRepoAuthRouter(t, fb, fetch, fakeClient)
	seedWebApp(t, db)
	var gotTokens []string
	rt.repoLayoutSource = func(_ context.Context, _, _, token string, _ repolayout.Hosts) (repolayout.Source, error) {
		gotTokens = append(gotTokens, token)
		return &fakeLayoutSource{files: map[string]string{"Dockerfile": "FROM scratch\n"}}, nil
	}
	const plaintext = "deploy-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/git-source/plan", strings.NewReader(`{"repo_url":"https://github.com/acme/private.git"}`))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(gotTokens) != 1 || gotTokens[0] != "" || fakeClient.gotInstallID != 0 {
		t.Fatalf("deploy-only plan used an app token: tokens=%q install=%d", gotTokens, fakeClient.gotInstallID)
	}
}

func TestProtectedEnvironmentOf_MissingAppIsNotAnError(t *testing.T) {
	rt, _ := newTestRouter(t)
	if _, found, err := rt.protectedEnvironmentOf(context.Background(), "missing"); found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestFreezeOverrideNote_SkipsHoldIfFrozen(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, nil)
	if _, err := db.ReplaceDeployFreezeWindows(context.Background(), store.DeployFreezeScopeApp("web"), []store.DeployFreezeWindow{{Cron: "* * * * *", Duration: 2 * time.Hour}}); err != nil {
		t.Fatal(err)
	}
	ctx := withFreezeOverrideNote(context.Background(), "FreezeOverride: hotfix")
	if held, _ := rt.holdIfFrozen(ctx, "web", deploy.HeldRequest{Kind: deploy.HeldKindGit}, "web:x"); held {
		t.Fatal("an accepted override must not be parked by holdIfFrozen")
	}
	if held, _ := rt.holdIfFrozen(context.Background(), "web", deploy.HeldRequest{Kind: deploy.HeldKindGit}, "web:x"); !held {
		t.Fatal("without an override the frozen deploy must be held")
	}
}

func TestKeepStoredDomains_KeepsAttachedVolumes(t *testing.T) {
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Volumes: []store.ServiceVolume{{Name: "web-data", ContainerPath: "/data"}},
	}); err != nil {
		t.Fatal(err)
	}
	desired := store.DesiredService{Name: "web", Image: "web:abc"}
	if err := rt.keepStoredDomains(context.Background(), &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Volumes) != 1 || desired.Volumes[0].ContainerPath != "/data" {
		t.Fatalf("volumes dropped: %+v", desired.Volumes)
	}
}
