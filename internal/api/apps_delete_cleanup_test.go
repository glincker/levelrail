package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeleteAppRemovesPerAppAccessRows(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now().UTC()

	seedApp(t, db, "site")
	if err := db.SavePolicy(ctx, store.Policy{ID: "pol_cleanup", Name: "ci-cleanup", Document: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	if err := db.SaveCIOIDCTrust(ctx, store.CIOIDCTrust{ID: "cit_cleanup", ServiceName: "site", Repository: "org/repo", RefPattern: "refs/heads/*", PolicyID: "pol_cleanup", CreatedAt: now}); err != nil {
		t.Fatalf("save trust: %v", err)
	}
	if err := db.CreateBuildHook(ctx, store.BuildHook{ID: "bh_cleanup", ServiceName: "site", Name: "main", TokenHash: "h", CreatedAt: now}); err != nil {
		t.Fatalf("create hook: %v", err)
	}
	if err := db.SetImageAutoUpdate(ctx, "site", true); err != nil {
		t.Fatalf("auto update: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/site", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	seedApp(t, db, "site")

	if trusts, err := db.ListCIOIDCTrusts(ctx, "site"); err != nil || len(trusts) != 0 {
		t.Errorf("trusts = %v, err = %v", trusts, err)
	}
	if _, err := db.GetPolicy(ctx, "pol_cleanup"); err == nil {
		t.Error("policy still exists")
	}
	if hooks, err := db.ListBuildHooks(ctx, "site"); err != nil || len(hooks) != 0 {
		t.Errorf("hooks = %v, err = %v", hooks, err)
	}
	if au, err := db.GetImageAutoUpdate(ctx, "site"); err != nil || au.Enabled {
		t.Errorf("auto update = %+v, err = %v", au, err)
	}
}

func TestDeleteAppRemovesGitSourceAndSecrets(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	mgr := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(mgr))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now().UTC()

	seedApp(t, db, "site")
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "site", RepoURL: "https://example.test/org/repo.git", Branch: "main"}); err != nil {
		t.Fatalf("save git source: %v", err)
	}
	gitKey := store.GitSourceSecretsKey("site")
	for _, kv := range [][2]string{{gitKey, "deploy_token"}, {gitKey, "webhook_secret"}, {"site", "APP_KEY"}} {
		if err := mgr.SetValue(ctx, kv[0], kv[1], "value-"+kv[1]); err != nil {
			t.Fatalf("set %v: %v", kv, err)
		}
	}
	p, err := db.SavePipeline(ctx, store.Pipeline{ID: "pl_cleanup", AppName: "site", Name: "ci", Source: "ui", YAML: "{}", Enabled: true, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("save pipeline: %v", err)
	}
	if err := db.SavePipelineWebhookTrigger(ctx, store.PipelineWebhookTrigger{PipelineID: p.ID, TokenHash: "h", Enabled: true, CreatedAt: now, RotatedAt: now}); err != nil {
		t.Fatalf("save trigger: %v", err)
	}
	hookKey := store.PipelineWebhookSecretsKey(p.ID)
	if err := mgr.SetValue(ctx, hookKey, "hmac", "value-hmac"); err != nil {
		t.Fatalf("set hook secret: %v", err)
	}
	if _, err := db.RotateFunctionKVToken(ctx, "site", now); err != nil {
		t.Fatalf("kv token: %v", err)
	}
	integKey := store.AppIntegrationSecretsKey("site", "metrics")
	if err := db.SaveAppIntegration(ctx, store.AppIntegration{ID: "ai_cleanup", ServiceName: "site", IntegrationKey: "metrics", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("save integration: %v", err)
	}
	prKey := store.PRPreviewEnvSecretsKey("site", 7)
	if _, err := db.SetPRPreviewEnvOverride(ctx, "site", 7, "API_KEY", "", true); err != nil {
		t.Fatalf("save pr override: %v", err)
	}
	for _, kv := range [][2]string{{integKey, "token"}, {prKey, "API_KEY"}} {
		if err := mgr.SetValue(ctx, kv[0], kv[1], "value-"+kv[1]); err != nil {
			t.Fatalf("set %v: %v", kv, err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/site", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	seedApp(t, db, "site")

	if _, err := db.GetGitSource(ctx, "site"); !errors.Is(err, store.ErrGitSourceNotFound) {
		t.Errorf("git source err = %v, want not found", err)
	}
	for _, kv := range [][2]string{{gitKey, "deploy_token"}, {gitKey, "webhook_secret"}, {"site", "APP_KEY"}, {hookKey, "hmac"}, {integKey, "token"}, {prKey, "API_KEY"}} {
		if ok, err := mgr.Exists(ctx, kv[0], kv[1]); err != nil || ok {
			t.Errorf("secret %v exists = %v, err = %v", kv, ok, err)
		}
	}
	if _, err := db.GetPipelineWebhookTrigger(ctx, p.ID); err == nil {
		t.Error("pipeline webhook trigger still exists")
	}
	if ok, err := db.HasFunctionKVToken(ctx, "site"); err != nil || ok {
		t.Errorf("kv token = %v, err = %v", ok, err)
	}
}

func TestDeleteGitSourceRouteErasesItsSecrets(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	mgr := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(mgr))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	seedApp(t, db, "site")
	if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "site", RepoURL: "https://example.test/org/repo.git", Branch: "main"}); err != nil {
		t.Fatalf("save git source: %v", err)
	}
	gitKey := store.GitSourceSecretsKey("site")
	for _, k := range []string{"deploy_token", "webhook_secret"} {
		if err := mgr.SetValue(ctx, gitKey, k, "value-"+k); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
	}
	if err := mgr.SetValue(ctx, "site", "APP_KEY", "keep"); err != nil {
		t.Fatalf("set app secret: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/site/git-source", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, k := range []string{"deploy_token", "webhook_secret"} {
		if ok, err := mgr.Exists(ctx, gitKey, k); err != nil || ok {
			t.Errorf("secret %s exists = %v, err = %v", k, ok, err)
		}
	}
	if ok, err := mgr.Exists(ctx, "site", "APP_KEY"); err != nil || !ok {
		t.Errorf("app secret exists = %v, err = %v", ok, err)
	}
}

func TestBuildSourceFactsScopedCallerGetsNoAppToken(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	seedApp(t, db, "site")
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "site", RepoURL: "https://github.com/org/repo.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatalf("save git source: %v", err)
	}
	var got []string
	rt.repoLayoutSource = func(_ context.Context, _, _, token string, _ repolayout.Hosts) (repolayout.Source, error) {
		got = append(got, token)
		return nil, errors.New("stop")
	}
	ctx := context.WithValue(context.Background(), teamScopeKey{}, &teamScope{projectID: "prj_other"})
	rt.addBuildSourceFacts(ctx, "site", &diagnose.Facts{})
	if len(got) != 1 || got[0] != "" {
		t.Errorf("tokens = %q, want one empty token", got)
	}
	if gitHubAppTokenAllowed(ctx) {
		t.Error("scoped caller allowed an app token")
	}
}
