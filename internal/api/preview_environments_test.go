package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// sequencedBuilder is a hand-written Builder fake whose Deploy calls
// return errs[i] for the i-th call (nil once i runs off the end of
// errs): builds_test.go's own fakeBuilder returns one fixed error for
// every call, which can't express "fails once, then succeeds on retry,"
// exactly the shape deployPreviewSingle's own domain-conflict fallback
// needs a test for. Unlike that fake, this one also writes through to a
// real *store.DB on success (SaveDesiredService, and for DeploySpec,
// SaveApp/UpdateServiceApp too), the same desired-state side effect the
// real internal/deploy.Pipeline has: preview_environments.go's own
// ensureAppLinked/tagPreviewWithEnvironment/teardownPreviewApp all read
// that state back, so a fake that only records calls without persisting
// anything would make every one of those steps fail against an empty
// database.
type sequencedBuilder struct {
	db        *store.DB
	tag       string
	errs      []error
	calls     []deploy.Request
	multiErr  error
	multiOuts []deploy.ServiceOutcome
}

func (s *sequencedBuilder) Deploy(ctx context.Context, req deploy.Request, _ func(build.ProgressEvent)) (string, error) {
	idx := len(s.calls)
	s.calls = append(s.calls, req)
	if idx < len(s.errs) && s.errs[idx] != nil {
		return "", s.errs[idx]
	}
	if s.db != nil {
		if err := s.db.SaveDesiredService(ctx, store.DesiredService{
			Name: req.ServiceName, Image: s.tag, Port: req.Service.Port, Domains: req.Service.Domains,
		}); err != nil {
			return "", err
		}
	}
	return s.tag, nil
}

func (s *sequencedBuilder) DeploySpec(ctx context.Context, req deploy.MultiRequest, progress func(string, build.ProgressEvent)) ([]deploy.ServiceOutcome, error) {
	if s.multiErr != nil {
		return nil, s.multiErr
	}
	if s.multiOuts != nil {
		return s.multiOuts, nil
	}

	if s.db != nil {
		if _, err := s.db.GetAppByName(ctx, req.AppName); errors.Is(err, store.ErrAppNotFound) {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if err := s.db.SaveApp(ctx, store.App{ID: req.AppName, Name: req.AppName, CreatedAt: now, UpdatedAt: now}); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
	}

	keys := make([]string, 0, len(req.Services))
	for k := range req.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]deploy.ServiceOutcome, 0, len(keys))
	for _, k := range keys {
		svcName := req.AppName + "-" + k
		if progress != nil {
			progress(k, build.ProgressEvent{Step: "fake", Completed: true})
		}
		if s.db != nil {
			svcSpec := req.Services[k]
			if err := s.db.SaveDesiredService(ctx, store.DesiredService{Name: svcName, Image: s.tag, Port: svcSpec.Port, Domains: svcSpec.Domains}); err != nil {
				out = append(out, deploy.ServiceOutcome{ServiceKey: k, ServiceName: svcName, Err: err})
				continue
			}
			if err := s.db.UpdateServiceApp(ctx, svcName, req.AppName); err != nil {
				out = append(out, deploy.ServiceOutcome{ServiceKey: k, ServiceName: svcName, Err: err})
				continue
			}
		}
		out = append(out, deploy.ServiceOutcome{ServiceKey: k, ServiceName: svcName, Image: s.tag})
	}
	return out, nil
}

// failingDeleteAppStore wraps a real AppStore, failing
// DeleteDesiredService for exactly one service name: the seam
// TestTeardownPreviewApp_PartialFailure uses to force
// teardownPreviewApp's own half-succeeded path without a full hand-
// written AppStore fake.
type failingDeleteAppStore struct {
	AppStore
	failName string
}

func (f *failingDeleteAppStore) DeleteDesiredService(ctx context.Context, name string) error {
	if name == f.failName {
		return errors.New("boom: delete failed")
	}
	return f.AppStore.DeleteDesiredService(ctx, name)
}

// githubPullRequestBody builds a pull_request event payload. headRef is
// always "feature-x" across this file's tests: only the action, number,
// commit, and base branch ever vary, so it's hardcoded rather than a
// parameter every call site would just repeat identically.
func githubPullRequestBody(action string, number int, headSHA, baseRef string) []byte {
	b, _ := json.Marshal(map[string]any{
		"action": action,
		"number": number,
		"pull_request": map[string]any{
			"head": map[string]any{"ref": "feature-x", "sha": headSHA},
			"base": map[string]any{"ref": baseRef},
		},
	})
	return b
}

// sendPullRequestWebhook posts body to app "web"'s webhook route: every
// test in this file exercises that one seeded app, the same fixed-name
// convention seedApp's own callers across this package already use.
func sendPullRequestWebhook(rt *Router, secret string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", sign([]byte(secret), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func setPreviewEnabled(t *testing.T, rt *Router, cookie *http.Cookie, appName string, enabled bool) {
	t.Helper()
	rec := httptest.NewRecorder()
	body := fmt.Sprintf(`{"enabled":%t}`, enabled)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/"+appName+"/preview-settings", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("set preview enabled: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// setUpPreviewApp seeds app "web", connects a git source, enables
// previews, and returns the generated webhook secret plus a ready-to-use
// sequencedBuilder wired to rt.builder. Callers needing their own
// authenticated session (e.g. to hit GET .../previews) mint one fresh
// via loginTestSession rather than reusing this one internal to the
// setup, so it stays a local variable, not a return value.
func setUpPreviewApp(t *testing.T) (rt *Router, db *store.DB, secret string, builder *sequencedBuilder) {
	t.Helper()
	secrets := newFakeGitSourceSecrets()
	rt, db = newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	setPreviewEnabled(t, rt, cookie, "web", true)

	builder = &sequencedBuilder{db: db, tag: "levelrail/web-pr-42:sha1"}
	rt.builder = builder
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)

	return rt, db, created.WebhookSecret, builder
}

func TestHandlePullRequestWebhook_PreviewNotEnabled_Ignored(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	// Preview environments left disabled (the default).

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, created.WebhookSecret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	previews, err := db.ListPreviewEnvironmentsByApp(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListPreviewEnvironmentsByApp() error = %v", err)
	}
	if len(previews) != 0 {
		t.Fatalf("ListPreviewEnvironmentsByApp() = %d rows, want 0 when previews are disabled", len(previews))
	}
}

func TestHandlePullRequestWebhook_Opened_CreatesPreview(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(builder.calls) != 1 {
		t.Fatalf("Deploy called %d times, want 1", len(builder.calls))
	}
	if builder.calls[0].ServiceName != "web-pr-42" {
		t.Errorf("ServiceName = %q, want %q", builder.calls[0].ServiceName, "web-pr-42")
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	if preview.Status != store.PreviewStatusActive {
		t.Errorf("Status = %q, want %q", preview.Status, store.PreviewStatusActive)
	}
	if preview.PreviewAppID != "web-pr-42" {
		t.Errorf("PreviewAppID = %q, want %q", preview.PreviewAppID, "web-pr-42")
	}
	if preview.HeadSHA != "sha1" {
		t.Errorf("HeadSHA = %q, want %q", preview.HeadSHA, "sha1")
	}

	svc, err := db.GetDesiredService(context.Background(), "web-pr-42")
	if err != nil {
		t.Fatalf("preview service was not created: %v", err)
	}
	if svc.AppID != "web-pr-42" {
		t.Errorf("preview service AppID = %q, want %q", svc.AppID, "web-pr-42")
	}
}

func TestHandlePullRequestWebhook_Synchronize_RedeploysExistingPreview(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)

	opened := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	syncEv := githubPullRequestBody("synchronize", 42, "sha2", "main")
	if rec := sendPullRequestWebhook(rt, secret, syncEv); rec.Code != http.StatusOK {
		t.Fatalf("synchronize: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if len(builder.calls) != 2 {
		t.Fatalf("Deploy called %d times, want 2", len(builder.calls))
	}

	previews, err := db.ListPreviewEnvironmentsByApp(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListPreviewEnvironmentsByApp() error = %v", err)
	}
	if len(previews) != 1 {
		t.Fatalf("ListPreviewEnvironmentsByApp() = %d rows, want 1 (reused, not duplicated)", len(previews))
	}
	if previews[0].HeadSHA != "sha2" {
		t.Errorf("HeadSHA = %q, want %q (the synchronize commit)", previews[0].HeadSHA, "sha2")
	}
}

func TestHandlePullRequestWebhook_WrongTargetBranch_Ignored(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)

	body := githubPullRequestBody("opened", 42, "sha1", "release")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(builder.calls) != 0 {
		t.Fatalf("Deploy called %d times, want 0 for a pull request against a non-target branch", len(builder.calls))
	}
}

func TestHandlePullRequestWebhook_Closed_TearsDownPreview(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)

	opened := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, opened); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42"); err != nil {
		t.Fatalf("preview service was not created: %v", err)
	}

	closed := githubPullRequestBody("closed", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, closed)
	if rec.Code != http.StatusOK {
		t.Fatalf("closed: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if _, err := db.GetDesiredService(context.Background(), "web-pr-42"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Errorf("preview service GetDesiredService() error = %v, want ErrServiceNotFound after teardown", err)
	}
	if _, err := db.GetAppByName(context.Background(), "web-pr-42"); !errors.Is(err, store.ErrAppNotFound) {
		t.Errorf("preview app GetAppByName() error = %v, want ErrAppNotFound after teardown", err)
	}
	if _, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42); !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		t.Errorf("GetPreviewEnvironmentByAppAndPR() error = %v, want ErrPreviewEnvironmentNotFound after teardown", err)
	}
}

func TestHandlePullRequestWebhook_ClosedWithNoPreview_Ignored(t *testing.T) {
	rt, _, secret, _ := setUpPreviewApp(t)

	closed := githubPullRequestBody("closed", 99, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, closed)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}
