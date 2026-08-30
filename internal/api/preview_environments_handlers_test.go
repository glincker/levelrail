package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestDeployPreviewEnvironment_DomainConflict_PartialSuccess is the
// "domain assignment fails" half-succeeded case: a base domain is
// configured, so the preview requests one, but the domain is already
// claimed elsewhere. The preview must still come up, just without that
// domain, and the API must report it as a 207 with a StatusReason, not
// a hard 500.
func TestDeployPreviewEnvironment_DomainConflict_PartialSuccess(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	ctx := context.Background()

	settings, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	settings.PrimaryDomain = "example.com"
	if err := db.UpdateIngressSettings(ctx, settings); err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}

	builder.errs = []error{&store.ErrDomainTaken{Domain: "pr-42.web.example.com", Owner: "someone-else"}}

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}

	if len(builder.calls) != 2 {
		t.Fatalf("Deploy called %d times, want 2 (initial attempt + no-domain retry)", len(builder.calls))
	}
	if len(builder.calls[0].Service.Domains) == 0 {
		t.Errorf("first Deploy call had no domain, want the requested preview domain")
	}
	if len(builder.calls[1].Service.Domains) != 0 {
		t.Errorf("retry Deploy call had domains %v, want none", builder.calls[1].Service.Domains)
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if preview.Status != store.PreviewStatusActive {
		t.Errorf("Status = %q, want %q (still usable, just domain-less)", preview.Status, store.PreviewStatusActive)
	}
	if preview.Domain != "" {
		t.Errorf("Domain = %q, want empty after a collision", preview.Domain)
	}
	if preview.StatusReason == "" {
		t.Errorf("StatusReason is empty, want an explanation of the domain collision")
	}

	if _, err := db.GetDesiredService(ctx, "web-pr-42"); err != nil {
		t.Fatalf("preview service was not created despite the domain conflict: %v", err)
	}
}

// TestTeardownPreviewApp_PartialFailure is the "teardown partially
// fails" half-succeeded case: one of two fanned-out services refuses to
// delete. The preview_environments row must survive (not be deleted) so
// the manual teardown action has something to retry, and the API must
// report 207, not a silent 200 or a full 500.
func TestTeardownPreviewApp_PartialFailure(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{
		"repo_url":"https://github.com/org/web.git",
		"branch":"main",
		"services": {"web": {"build": {"type": "dockerfile"}, "port": 3000}, "worker": {"build": {"type": "dockerfile"}}}
	}`)
	setPreviewEnabled(t, rt, cookie, "web", true)

	builder := &sequencedBuilder{db: db, tag: "levelrail/web-pr-42:sha1"}
	rt.builder = builder
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)

	opened := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, created.WebhookSecret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42-web"); err != nil {
		t.Fatalf("fanned-out preview service not created: %v", err)
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42-worker"); err != nil {
		t.Fatalf("fanned-out preview service not created: %v", err)
	}

	rt.apps = &failingDeleteAppStore{AppStore: db, failName: "web-pr-42-worker"}

	closed := githubPullRequestBody("closed", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, created.WebhookSecret, closed)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusMultiStatus, rec.Body.String())
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v, want the row to survive a partial teardown", err)
	}
	if preview.Status != store.PreviewStatusFailed {
		t.Errorf("Status = %q, want %q", preview.Status, store.PreviewStatusFailed)
	}
	if preview.StatusReason == "" {
		t.Errorf("StatusReason is empty, want an explanation naming the undeleted service")
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42-web"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Errorf("web-pr-42-web GetDesiredService() error = %v, want ErrServiceNotFound (it should have deleted fine)", err)
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42-worker"); err != nil {
		t.Errorf("web-pr-42-worker GetDesiredService() error = %v, want the service to still exist after its own delete failed", err)
	}

	// Retrying with the failure lifted (the manual teardown action's own
	// real-world use case) must finish the job.
	rt.apps = db
	rec = rt.teardownAndRecord(t, "web", 42)
	if rec.Code != http.StatusOK {
		t.Fatalf("retry teardown: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42); !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		t.Errorf("GetPreviewEnvironmentByAppAndPR() after retry error = %v, want ErrPreviewEnvironmentNotFound", err)
	}
}

// teardownAndRecord drives POST .../previews/{number}/teardown directly
// (the manual teardown action), reusing whatever cookie-less router
// state the caller already set up: the webhook path already exercises
// closed-event teardown, so this helper covers the HTTP-handler path
// specifically.
func (rt *Router) teardownAndRecord(t *testing.T, appName string, prNumber int) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/apps/%s/previews/%d/teardown", appName, prNumber), nil)
	req.SetPathValue("name", appName)
	req.SetPathValue("number", fmt.Sprintf("%d", prNumber))
	rt.handleTeardownPreviewEnvironment(rec, req)
	return rec
}

func TestHandleListPreviewEnvironments(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, body); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/previews", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got []previewEnvironmentResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].PRNumber != 42 {
		t.Errorf("PRNumber = %d, want 42", got[0].PRNumber)
	}
}

func TestHandleListPreviewEnvironments_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/previews", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "[]" && body != "[]\n" {
		t.Errorf("body = %q, want an empty JSON array", body)
	}
}

func TestHandleSetPreviewEnabled_NoGitSource(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{"enabled":true}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleTeardownPreviewEnvironment_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/1/teardown", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
