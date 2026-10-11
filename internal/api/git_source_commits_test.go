package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCommitSource struct {
	fakeLayoutSource
	commits    []repolayout.Commit
	branches   []string
	commitsErr error
	calls      int
}

func (f *fakeCommitSource) Provider() string { return repolayout.ProviderGitHub }

func (f *fakeCommitSource) Commits(context.Context, int) ([]repolayout.Commit, error) {
	f.calls++
	return f.commits, f.commitsErr
}

func (f *fakeCommitSource) Branches(context.Context) ([]string, error) { return f.branches, nil }

func commitsFixture() *fakeCommitSource {
	now := time.Now()
	return &fakeCommitSource{
		fakeLayoutSource: fakeLayoutSource{files: map[string]string{"Dockerfile": "FROM scratch"}},
		commits: []repolayout.Commit{
			{SHA: "c3c3c3c3c3", Message: "feat: new", Author: "Ann", AuthoredAt: now},
			{SHA: "b2b2b2b2b2", Message: "fix: bug", Author: "Bob", AuthoredAt: now.Add(-time.Hour)},
			{SHA: "a1a1a1a1a1", Message: "init", Author: "Cy", AuthoredAt: now.Add(-2 * time.Hour)},
		},
		branches: []string{"main", "dev"},
	}
}

func seedAttempt(t *testing.T, db *store.DB, id, sha, status, image string, ago time.Duration) {
	t.Helper()
	fin := time.Now().Add(-ago + time.Minute)
	err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: id, ServiceName: "web", Image: image, CommitSHA: sha,
		Source: store.DeployAttemptSourceWebhook, Status: status,
		StartedAt: time.Now().Add(-ago), FinishedAt: &fin,
	})
	if err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
}

func TestGitSourceCommits_AheadByAndShortcuts(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/app.git","branch":"main","build_type":"dockerfile"}`)
	src := commitsFixture()
	useFakeLayout(rt, src)

	seedAttempt(t, db, "dep_old", "a1a1a1a1a1", store.DeployAttemptStatusSucceeded, "app:a1a1", 3*time.Hour)
	seedAttempt(t, db, "dep_dep", "b2b2b2b2b2", store.DeployAttemptStatusSucceeded, "app:b2b2", 2*time.Hour)
	seedAttempt(t, db, "dep_bad", "c3c3c3c3c3", store.DeployAttemptStatusFailed, "app:c3c3", time.Hour)

	rec := layoutReq(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/git-source/commits", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got gitSourceCommitsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Head == nil || got.Head.SHA != "c3c3c3c3c3" || got.Head.DeployStatus != "failed" {
		t.Fatalf("head = %+v", got.Head)
	}
	if got.AheadBy == nil || *got.AheadBy != 1 || got.UpToDate {
		t.Fatalf("ahead_by = %v up_to_date = %v", got.AheadBy, got.UpToDate)
	}
	if got.Deployed == nil || got.Deployed.CommitSHA != "b2b2b2b2b2" {
		t.Fatalf("deployed = %+v", got.Deployed)
	}
	if got.LastSuccessful == nil || got.PreviousSuccessful == nil || got.PreviousSuccessful.Image != "app:a1a1" {
		t.Fatalf("shortcuts last=%+v prev=%+v", got.LastSuccessful, got.PreviousSuccessful)
	}
	if len(got.Branches) != 2 || got.Build.DockerfilePath != "Dockerfile" {
		t.Fatalf("branches=%v build=%+v", got.Branches, got.Build)
	}
	var ids []string
	for _, s := range got.Suggestions {
		ids = append(ids, s.ID)
	}
	if !strings.Contains(strings.Join(ids, ","), suggestHeadFailed) {
		t.Fatalf("expected head_failed suggestion, got %v", ids)
	}

	layoutReq(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/git-source/commits", "")
	if src.calls != 1 {
		t.Fatalf("provider called %d times, want 1 (cached)", src.calls)
	}
	layoutReq(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/git-source/commits?refresh=1", "")
	if src.calls != 2 {
		t.Fatalf("refresh did not bypass cache, calls = %d", src.calls)
	}
}

func TestGitSourceCommits_Errors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		target     string
		wantStatus int
		wantMsg    string
	}{
		{"not found", &repolayout.StatusError{Status: 404, Body: "secret-token-abc"}, "", http.StatusNotFound, "not found"},
		{"forbidden", &repolayout.StatusError{Status: 403, Body: "secret-token-abc"}, "", http.StatusUnprocessableEntity, "refused access"},
		{"rate limited", &repolayout.StatusError{Status: 429, Body: "secret-token-abc"}, "", http.StatusTooManyRequests, "rate limiting"},
		{"other", errors.New("dial tcp: secret-token-abc"), "", http.StatusBadGateway, "could not read commits"},
		{"bad branch", nil, "?branch=--upload-pack", http.StatusBadRequest, "valid branch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/app.git","branch":"main","build_type":"dockerfile"}`)
			src := commitsFixture()
			src.commitsErr = tc.err
			useFakeLayout(rt, src)

			rec := layoutReq(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/git-source/commits"+tc.target, "")
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.wantMsg) {
				t.Fatalf("body %q lacks %q", body, tc.wantMsg)
			}
			if strings.Contains(body, "secret-token-abc") {
				t.Fatalf("provider detail leaked: %s", body)
			}
		})
	}
}

func TestGitSourceCommits_NoSource(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := layoutReq(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/git-source/commits", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestComputeDeploySuggestions(t *testing.T) {
	healthy := suggestionInput{WebhookSeen: true, HasHealthCheck: true, HasNotifyTarget: true, PreviewEnabled: true, Provider: "github", TriggerMode: "push"}
	tests := []struct {
		name   string
		mutate func(*suggestionInput)
		want   []string
	}{
		{"all good", func(*suggestionInput) {}, nil},
		{"no webhook", func(i *suggestionInput) { i.WebhookSeen = false; i.PreviewEnabled = true }, []string{suggestWebhookMissing}},
		{"bad signature", func(i *suggestionInput) { i.WebhookBadSignature = true }, []string{suggestWebhookSignature}},
		{"release mode", func(i *suggestionInput) { i.TriggerMode = "release" }, []string{suggestReleaseOnly}},
		{"monorepo", func(i *suggestionInput) { i.LooksLikeMonorepo = true }, []string{suggestBuildSettings}},
		{"monorepo with base dir set", func(i *suggestionInput) { i.LooksLikeMonorepo = true; i.BaseDirectory = "apps/web" }, nil},
		{"build config failure", func(i *suggestionInput) { i.LastFailureCode = "dockerfile_error" }, []string{suggestBuildSettings}},
		{"unrelated failure", func(i *suggestionInput) { i.LastFailureCode = "compile_error" }, nil},
		{"previews", func(i *suggestionInput) { i.PreviewEnabled = false }, []string{suggestPreviews}},
		{"previews unsupported provider", func(i *suggestionInput) { i.PreviewEnabled = false; i.Provider = "git" }, nil},
		{"no health", func(i *suggestionInput) { i.HasHealthCheck = false }, []string{suggestHealthCheck}},
		{"no notifications", func(i *suggestionInput) { i.HasNotifyTarget = false }, []string{suggestNotifications}},
		{"head failed first", func(i *suggestionInput) { i.HeadFailedDeployID = "dep_x"; i.HasHealthCheck = false }, []string{suggestHeadFailed, suggestHealthCheck}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := healthy
			tc.mutate(&in)
			var got []string
			for _, s := range computeDeploySuggestions(in) {
				got = append(got, s.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidBranchName(t *testing.T) {
	for b, want := range map[string]bool{"main": true, "feat/x-1": true, "-x": false, "a..b": false, "a b": false, "": false, "a:b": false} {
		if validBranchName(b) != want {
			t.Errorf("validBranchName(%q) = %v", b, !want)
		}
	}
}
