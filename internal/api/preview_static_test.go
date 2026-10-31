package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticPreviewBuilder publishes a one-file tree under <root>/<service>/<sha>
// and saves its site row, the same effect deploy.Pipeline has for a static app.
type staticPreviewBuilder struct {
	db    *store.DB
	root  string
	calls []deploy.Request
}

func (b *staticPreviewBuilder) Deploy(ctx context.Context, req deploy.Request, _ func(build.ProgressEvent)) (string, error) {
	b.calls = append(b.calls, req)
	dir := filepath.Join(b.root, req.ServiceName, req.CommitSHA)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // temp dir in a test
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(req.CommitSHA), 0o600); err != nil { //nolint:gosec // temp dir in a test
		return "", err
	}
	return dir, b.db.SaveStaticSite(ctx, store.StaticSite{Name: req.ServiceName, Domains: req.Service.Domains, RootDir: dir})
}

func (b *staticPreviewBuilder) DeploySpec(context.Context, deploy.MultiRequest, func(string, build.ProgressEvent)) ([]deploy.ServiceOutcome, error) {
	return nil, nil
}

// setUpStaticPreviewApp seeds a live static site "web" (row plus files) and a
// source whose previews are enabled and report to a fake GitHub client.
func setUpStaticPreviewApp(t *testing.T) (rt *Router, db *store.DB, secret string, builder *staticPreviewBuilder, gh *fakeGitHubAppClient, mainDir string) {
	t.Helper()
	githubSecrets := newFakeGitHubAppSecrets()
	gh = &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: "ghs_installtoken"}}
	rt, db = newTestRouterWithGitHubAndGitSource(t, githubSecrets, gh, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, githubSecrets)
	seedApp(t, db, "web")

	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"static"}`)
	setPreviewEnabled(t, rt, cookie)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview-settings", `{"post_pr_comments":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("set post_pr_comments: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	root := t.TempDir()
	mainDir = filepath.Join(root, "web", "live1")
	if err := os.MkdirAll(mainDir, 0o750); err != nil {
		t.Fatalf("mkdir main site: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "index.html"), []byte("live"), 0o600); err != nil {
		t.Fatalf("write main site: %v", err)
	}
	if err := db.SaveStaticSite(context.Background(), store.StaticSite{Name: "web", RootDir: mainDir}); err != nil {
		t.Fatalf("seed main site: %v", err)
	}

	builder = &staticPreviewBuilder{db: db, root: root}
	rt.builder = builder
	rt.staticRootDir = root
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)
	return rt, db, created.WebhookSecret, builder, gh, mainDir
}

func assertMainSiteIntact(t *testing.T, db *store.DB, mainDir string) {
	t.Helper()
	site, err := db.GetStaticSite(context.Background(), "web")
	if err != nil || site.RootDir != mainDir {
		t.Fatalf("main site = %+v, err %v; want root %q untouched", site, err, mainDir)
	}
	if got, err := os.ReadFile(filepath.Join(mainDir, "index.html")); err != nil || string(got) != "live" { //nolint:gosec // temp dir in a test
		t.Fatalf("main site file = %q, err %v; want it untouched", got, err)
	}
}

func TestStaticPreview_PublishesSeparateRelease(t *testing.T) {
	rt, db, secret, builder, gh, mainDir := setUpStaticPreviewApp(t)
	ctx := context.Background()

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if len(builder.calls) != 1 {
		t.Fatalf("Deploy called %d times, want 1", len(builder.calls))
	}
	call := builder.calls[0]
	if call.ServiceName != "web-pr-42" || call.CommitSHA != "sha1" || !call.Preview || call.Service.Build.Type != spec.BuildStatic {
		t.Fatalf("deploy request = %+v, want a static preview build of sha1 as web-pr-42", call)
	}
	if _, err := db.GetStaticSite(ctx, "web-pr-42"); err != nil {
		t.Fatalf("preview site not saved: %v", err)
	}
	if _, err := db.GetDesiredService(ctx, "web-pr-42"); err == nil {
		t.Fatal("a static preview must not create a service row")
	}
	assertMainSiteIntact(t, db, mainDir)

	preview, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil || preview.Status != store.PreviewStatusActive {
		t.Fatalf("preview = %+v, err %v; want active", preview, err)
	}
	if len(gh.statusCalls) != 2 || gh.statusCalls[0].state != githubapp.CommitStatusPending || gh.statusCalls[1].state != githubapp.CommitStatusSuccess {
		t.Fatalf("commit statuses = %+v, want pending then success", gh.statusCalls)
	}
	if len(gh.commentCalls) != 1 || gh.commentCalls[0].number != 42 {
		t.Fatalf("comment calls = %+v, want one on pr 42", gh.commentCalls)
	}
}

func TestStaticPreview_OrphanSweepKeepsPreview(t *testing.T) {
	rt, db, secret, _, _, _ := setUpStaticPreviewApp(t)
	ctx := context.Background()
	if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main")); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rt.staticOrphanGrace = time.Nanosecond
	time.Sleep(5 * time.Millisecond)

	removed, err := rt.SweepOrphanStaticSites(ctx)
	if err != nil || len(removed) != 0 {
		t.Fatalf("orphan sweep removed %v, err %v; want nothing", removed, err)
	}
	if _, err := db.GetStaticSite(ctx, "web-pr-42"); err != nil {
		t.Fatalf("preview site gone after orphan sweep: %v", err)
	}
}

func TestStaticPreview_ContainerAppUnchanged(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)

	if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main")); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(builder.calls) != 1 || builder.calls[0].Service.Build.Type == spec.BuildStatic {
		t.Fatalf("calls = %+v, want one non-static deploy", builder.calls)
	}
	if _, err := db.GetDesiredService(context.Background(), "web-pr-42"); err != nil {
		t.Fatalf("container preview service missing: %v", err)
	}
	if _, err := db.GetStaticSite(context.Background(), "web-pr-42"); err == nil {
		t.Fatal("a container preview must not create a static site")
	}
}
