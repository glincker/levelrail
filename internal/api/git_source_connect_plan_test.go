package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

func recordPlanTokens(rt *Router) *[]string {
	var got []string
	rt.repoLayoutSource = func(_ context.Context, _, _, token string, _ repolayout.Hosts) (repolayout.Source, error) {
		got = append(got, token)
		return &fakeLayoutSource{files: map[string]string{"Dockerfile": "FROM scratch\n"}}, nil
	}
	return &got
}

func decodePlan(t *testing.T, rec *httptest.ResponseRecorder) gitSourcePlanResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var got gitSourcePlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestGitSourcePlan_ConnectionTokenReadsPrivateGitLab(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	gl := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.example.test", "jane", true)
	tokens := recordPlanTokens(rt)

	cases := []struct{ repo, conn, want string }{
		{"https://gitlab.example.test/g/private.git", gl.ID, "at-jane"},
		// A connection never travels to a host, port or scheme it was not made for.
		{"https://evil.example.test/g/private.git", gl.ID, ""},
		{"http://gitlab.example.test/g/private.git", gl.ID, ""},
		{"https://gitlab.example.test:8443/g/private.git", gl.ID, ""},
		{"https://user@gitlab.example.test/g/private.git", gl.ID, ""},
		{"https://gitlab.example.test./g/private.git", gl.ID, ""},
		{"https://gitlab.example.test/g/private.git", "missing", ""},
	}
	for _, c := range cases {
		*tokens = nil
		body := `{"repo_url":"` + c.repo + `","connection_id":"` + c.conn + `"}`
		decodePlan(t, layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/plan", body))
		if len(*tokens) != 1 || (*tokens)[0] != c.want {
			t.Errorf("%s via %s: tokens %q, want %q", c.repo, c.conn, *tokens, c.want)
		}
	}
}

func TestGitSourcePlan_DeployOnlyCallerNeverUsesConnectionToken(t *testing.T) {
	secrets := newFakeGitConnSecrets()
	rt, db := newGitConnTestRouter(t, secrets)
	seedApp(t, db, "web")
	gl := seedGitConn(t, db, secrets, gitProviderGitLab, "gitlab.example.test", "jane", true)
	tokens := recordPlanTokens(rt)
	const plaintext = "deploy-only-plan-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_deploy", Name: "deployer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityDeploy}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	body := `{"repo_url":"https://gitlab.example.test/g/private.git","connection_id":"` + gl.ID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/git-source/plan", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	got := decodePlan(t, rec)
	if len(*tokens) != 1 || (*tokens)[0] != "" {
		t.Fatalf("deploy-only plan used a connection token: %q", *tokens)
	}
	if !slices.Contains(got.Warnings, gitConnectWarnConnNeedsRead) {
		t.Fatalf("missing %s warning: %v", gitConnectWarnConnNeedsRead, got.Warnings)
	}
}

func TestGitSourcePlan_ListsGroupMembersAndEffects(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, n := range []string{"api", "worker", "db", "other"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "img:" + n, Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	rt.apps = groupApps{rt.apps}
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "worker", RepoURL: "https://github.com/org/worker.git", Branch: "main", BuildType: "railpack"}); err != nil {
		t.Fatal(err)
	}
	got := decodePlan(t, layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/api/git-source/plan", `{}`))
	want := []gitSourcePlanMember{
		{Name: "db", CurrentSource: gitConnectSourceImage, Effect: gitConnectMemberUnchanged},
		{Name: "worker", CurrentSource: gitConnectSourceGit, Effect: gitConnectMemberOwnSource},
	}
	if !slices.Equal(got.Group, want) {
		t.Fatalf("group = %+v, want %+v", got.Group, want)
	}
}

// groupApps puts every service but "other" in one app without app rows.
type groupApps struct{ AppStore }

func groupOf(s *store.DesiredService) {
	if s.Name != "other" {
		s.AppID = "shop"
	}
}

func (a groupApps) GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error) {
	svc, err := a.AppStore.GetDesiredService(ctx, name)
	if svc != nil {
		groupOf(svc)
	}
	return svc, err
}

func (a groupApps) ListDesiredServices(ctx context.Context) ([]store.DesiredService, error) {
	all, err := a.AppStore.ListDesiredServices(ctx)
	for i := range all {
		groupOf(&all[i])
	}
	return all, err
}

// failingEnvironments fails every read, the state where protection is unknown.
type failingEnvironments struct{ EnvironmentStore }

func (failingEnvironments) GetEnvironment(context.Context, string) (store.Environment, error) {
	return store.Environment{}, errors.New("environment store unavailable")
}

// envApps reports an environment on every service without a real row.
type envApps struct{ AppStore }

func (a envApps) GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error) {
	svc, err := a.AppStore.GetDesiredService(ctx, name)
	if svc != nil {
		svc.EnvironmentID = "env_prod"
	}
	return svc, err
}

func TestGitSourceDeploy_ProtectionUnknownFailsClosed(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, nil)
	fb := &fakeBuilder{tag: "web:sha"}
	rt.builder = fb
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "http://127.0.0.1:1/org/web.git", Branch: "main", BuildType: "railpack"}); err != nil {
		t.Fatal(err)
	}
	rt.apps = envApps{rt.apps}
	rt.environments = failingEnvironments{rt.environments}

	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/deploy", `{"override_freeze":true,"override_reason":"hotfix"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d %s, want 503", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "environment store unavailable") {
		t.Fatal("internal error text leaked to the caller")
	}
	rt.buildStartMu.Lock()
	marks := rt.startingDeploys["web"]
	rt.buildStartMu.Unlock()
	if marks != 0 {
		t.Fatalf("a refused deploy left %d start marks", marks)
	}

	got := decodePlan(t, serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/git-source/plan", `{}`)))
	if !slices.Contains(got.Warnings, gitConnectWarnProtectedUnknown) {
		t.Fatalf("plan hides unknown protection: %v", got.Warnings)
	}
}

func TestPlanGroupMembers_SkipsSiblingsTheCallerCannotRead(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	ctx := context.Background()
	for _, n := range []string{"api", "worker", "db"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "img:" + n, Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	rt.apps = groupApps{rt.apps}
	svc, err := rt.apps.GetDesiredService(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	got := rt.planGroupMembers(ctx, *svc, func(n string) bool { return n != "db" })
	if len(got) != 1 || got[0].Name != "worker" {
		t.Fatalf("hidden sibling listed: %+v", got)
	}
	if got := rt.planGroupMembers(ctx, *svc, func(string) bool { return false }); len(got) != 0 {
		t.Fatalf("no read access still lists %+v", got)
	}
}

func TestQueuedOverrideReplayRestoresFreezeNote(t *testing.T) {
	gs := store.GitSource{ServiceName: "web", Branch: "main"}
	queued := gatedBranchHeldRequest(withFreezeOverrideNote(context.Background(), "FreezeOverride: hotfix"), gs, "abc")
	raw, err := json.Marshal(queued)
	if err != nil {
		t.Fatal(err)
	}
	var saved deploy.HeldRequest
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if note := freezeOverrideNoteFrom(replayContext(context.Background(), saved)); note != "FreezeOverride: hotfix" {
		t.Fatalf("replayed deploy lost its override reason: %q", note)
	}
	if note := freezeOverrideNoteFrom(replayContext(context.Background(), deploy.HeldRequest{})); note != "" {
		t.Fatalf("replay invented a reason: %q", note)
	}
}

func TestGatedBranchHeldRequest_CarriesFreezeOverride(t *testing.T) {
	gs := store.GitSource{ServiceName: "web", Branch: "main"}
	ctx := withFreezeOverrideNote(context.Background(), "FreezeOverride: hotfix")
	if hr := gatedBranchHeldRequest(ctx, gs, "abc"); hr.FreezeNote != "FreezeOverride: hotfix" || hr.CommitSHA != "abc" || hr.Branch != "main" {
		t.Fatalf("queued request lost the override: %+v", hr)
	}
	if hr := gatedBranchHeldRequest(context.Background(), gs, "abc"); hr.FreezeNote != "" {
		t.Fatalf("note without an override: %q", hr.FreezeNote)
	}
}
