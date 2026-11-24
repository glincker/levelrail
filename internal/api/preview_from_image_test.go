package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func postImagePreview(t *testing.T, rt *Router, cookie *http.Cookie, pr string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews", `{"pr_number":`+pr+`,"image":"registry.example.com/org/web:abc123"}`))
	return rec
}

func TestHandleCreatePreviewFromImage_Refusals(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	ctx := context.Background()
	cookie := loginTestSession(t, rt, db)

	seedApp(t, db, "web-pr-9")
	if rec := postImagePreview(t, rt, cookie, "9"); rec.Code != http.StatusConflict {
		t.Fatalf("name collision status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main")); rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d", rec.Code)
	}
	if rec := postImagePreview(t, rt, cookie, "42"); rec.Code != http.StatusConflict {
		t.Fatalf("webhook preview status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	held := store.PreviewEnvironment{ID: "held1", AppName: "web", PRNumber: 11, PreviewAppID: "web-pr-11", Status: store.PreviewStatusAwaitingApproval, IsFork: true, HeadRepo: "someone/web", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := db.SavePreviewEnvironment(ctx, held); err != nil {
		t.Fatal(err)
	}
	if rec := postImagePreview(t, rt, cookie, "11"); rec.Code != http.StatusConflict {
		t.Fatalf("fork hold status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	failedRow := store.PreviewEnvironment{ID: "failed1", AppName: "web", PRNumber: 13, PreviewAppID: "web-pr-13", Status: store.PreviewStatusFailed, HeadRepo: "org/web", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := db.SavePreviewEnvironment(ctx, failedRow); err != nil {
		t.Fatal(err)
	}
	if rec := postImagePreview(t, rt, cookie, "13"); rec.Code != http.StatusConflict {
		t.Fatalf("failed webhook row status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	if err := db.SavePreviewAppSettings(ctx, store.PreviewAppSettings{AppName: "web", OnLimit: store.PreviewOnLimitEvictOldest, DatabaseStrategy: store.PreviewDatabaseStrategyFresh}); err != nil {
		t.Fatal(err)
	}
	if rec := postImagePreview(t, rt, cookie, "12"); rec.Code != http.StatusConflict {
		t.Fatalf("fresh database strategy status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveImagePreviewService_KeepsForeignAppGroup(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedApp(t, db, "web-pr-5")
	if err := db.SaveApp(ctx, store.App{ID: "web-pr-5", Name: "web-pr-5", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	rt.removeImagePreviewService(ctx, "web-pr-5", true)

	if _, err := db.GetDesiredService(ctx, "web-pr-5"); err == nil {
		t.Fatal("preview service still exists")
	}
	if _, err := db.GetAppByName(ctx, "web-pr-5"); err != nil {
		t.Fatalf("app group was deleted: %v", err)
	}
}

func TestHandleCreatePreviewFromImage_NoGitHostComment(t *testing.T) {
	githubSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db := newTestRouterWithGitHubAndGitSource(t, githubSecrets, fakeClient, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, githubSecrets)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	setPreviewEnabled(t, rt, cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{"post_pr_comments":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("set post_pr_comments: status = %d", rec.Code)
	}
	rt.builder = &sequencedBuilder{db: db, tag: "levelrail/web-pr-7:sha1"}

	if rec := postImagePreview(t, rt, cookie, "7"); rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/7/teardown", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("teardown status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(fakeClient.commentCalls) != 0 || len(fakeClient.prComments.updates) != 0 {
		t.Fatalf("git host saw %d comment creates and %d edits, want none", len(fakeClient.commentCalls), len(fakeClient.prComments.updates))
	}
}

func TestHandleCreatePreviewFromImage_NeverEvictsRealPreview(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	ctx := context.Background()
	if err := db.SavePreviewAppSettings(ctx, store.PreviewAppSettings{AppName: "web", OnLimit: store.PreviewOnLimitEvictOldest, MaxPreviews: 1, DatabaseStrategy: store.PreviewDatabaseStrategyNone}); err != nil {
		t.Fatal(err)
	}
	if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main")); rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews", `{"pr_number":7,"image":"registry.example.com/org/web:abc123"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	if p, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42); err != nil || p.Status != store.PreviewStatusActive {
		t.Fatalf("real preview = %+v, err = %v, want still active", p, err)
	}
}

func TestHandleCreatePreviewFromImage(t *testing.T) {
	rt, db, _, builder := setUpPreviewApp(t)
	cookie := loginTestSession(t, rt, db)
	code, ro := mintToken(t, rt, cookie, `{"name":"ci-readonly"}`)
	if code != http.StatusCreated {
		t.Fatalf("mint token status = %d", code)
	}
	body := `{"pr_number":7,"image":"registry.example.com/org/web:abc123","branch":"feat-x","sha":"abc123"}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodPost, "/api/v1/apps/web/previews", body, ro.Token))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("read-only token status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews", `{"pr_number":7,"image":"NOT A REF"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad image status = %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(builder.calls) != 1 || builder.calls[0].Service.Build.Type != spec.BuildImage || builder.calls[0].Service.Build.Image != "registry.example.com/org/web:abc123" {
		t.Fatalf("deploy calls = %+v, want one image deploy", builder.calls)
	}
	if p, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 7); err != nil || p.Status != "active" {
		t.Fatalf("preview row = %+v, err = %v", p, err)
	}
}
