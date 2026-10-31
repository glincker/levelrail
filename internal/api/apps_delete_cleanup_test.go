package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/site", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	seedApp(t, db, "site")

	if _, err := db.GetGitSource(ctx, "site"); !errors.Is(err, store.ErrGitSourceNotFound) {
		t.Errorf("git source err = %v, want not found", err)
	}
	for _, kv := range [][2]string{{gitKey, "deploy_token"}, {gitKey, "webhook_secret"}, {"site", "APP_KEY"}, {hookKey, "hmac"}} {
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
