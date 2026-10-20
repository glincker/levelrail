package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploycontext"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDeployContextRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/contexts"},
		{http.MethodPut, "/api/v1/apps/web/contexts"},
		{http.MethodDelete, "/api/v1/apps/web/contexts?context=deploy-preview"},
		{http.MethodGet, "/api/v1/apps/web/effective-config"},
	})
}

func ctxDo(t *testing.T, rt *Router, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestSetAndListAppContext(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	bad := []string{
		`{"context":"production","env":{"A":"1"}}`,
		`{"context":"staging"}`,
		`{"context":"deploy-preview","build_command":"make","publish_dir":"dist"}`,
	}
	for _, body := range bad {
		if rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/contexts", body, cookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT %s = %d, want 400 (%s)", body, rec.Code, rec.Body.String())
		}
	}

	ok := `{"context":"deploy-preview","env":{"API_URL":"https://stg","DB_PASSWORD":"hunter2"},"build_args":{"NODE":"22"}}`
	if rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/contexts", ok, cookie); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d (%s)", rec.Code, rec.Body.String())
	}
	rec := ctxDo(t, rt, http.MethodGet, "/api/v1/apps/web/contexts", "", cookie)
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("list leaked a sensitive value: %s", rec.Body.String())
	}
	var rows []contextConfigResource
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].Env["API_URL"] != "https://stg" {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}

	// Saving the masked list output back keeps the stored secret value.
	back, _ := json.Marshal(rows[0])
	if rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/contexts", string(back), cookie); rec.Code != http.StatusOK {
		t.Fatalf("round trip PUT = %d (%s)", rec.Code, rec.Body.String())
	}
	stored, _ := db.ListAppContextConfigs(context.Background(), "web")
	if stored[0].Env["DB_PASSWORD"] != "hunter2" {
		t.Fatalf("masked placeholder overwrote the stored value: %+v", stored[0].Env)
	}

	if rec := ctxDo(t, rt, http.MethodDelete, "/api/v1/apps/web/contexts?context=deploy-preview", "", cookie); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", rec.Code)
	}
	if rec := ctxDo(t, rt, http.MethodDelete, "/api/v1/apps/web/contexts?context=deploy-preview", "", cookie); rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE = %d, want 404", rec.Code)
	}
}

type failingContextStore struct{ ContextConfigStore }

func (failingContextStore) SaveAppContextConfig(context.Context, store.AppContextConfig) error {
	return errors.New("disk full")
}

func TestSetAppContext_SaveFailureLeavesStoredConfig(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	body := `{"context":"branch-deploy","env":{"A":"old"}}`
	if rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/contexts", body, cookie); rec.Code != http.StatusOK {
		t.Fatalf("seed PUT = %d", rec.Code)
	}
	rt.contextConfigs = failingContextStore{rt.contextConfigs}
	rec := ctxDo(t, rt, http.MethodPut, "/api/v1/apps/web/contexts", `{"context":"branch-deploy","env":{"A":"new"}}`, cookie)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "disk full") {
		t.Fatalf("PUT = %d (%s), want opaque 500", rec.Code, rec.Body.String())
	}
	stored, _ := db.ListAppContextConfigs(context.Background(), "web")
	if len(stored) != 1 || stored[0].Env["A"] != "old" {
		t.Fatalf("failed save changed stored config: %+v", stored)
	}
}

func TestEffectiveConfig(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	svc := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 8080,
		Env:       map[string]string{"A": "app", "B": "app"},
		SecretEnv: []store.SecretEnvRef{{Name: "TOKEN"}},
	}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	legacy := "legacy"
	if err := db.SetServicePreviewEnvOverride(ctx, "web", "B", &legacy); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAppContextConfig(ctx, store.AppContextConfig{AppName: "web", Context: "deploy-preview", Env: map[string]string{"A": "ctx", "API_KEY": "abc"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetServiceBranchEnvOverride(ctx, "web", "feat/*", "A", "branchy", false); err != nil {
		t.Fatal(err)
	}

	rec := ctxDo(t, rt, http.MethodGet, "/api/v1/apps/web/effective-config?context=deploy-preview&branch=feat/x", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "abc") {
		t.Fatalf("sensitive-named context value not masked: %s", rec.Body.String())
	}
	var eff deploycontext.Effective
	if err := json.Unmarshal(rec.Body.Bytes(), &eff); err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"A":       {"branchy", deploycontext.SourceBranchOverride},
		"B":       {"legacy", deploycontext.SourceLegacyPreview},
		"TOKEN":   {deploycontext.MaskedValue, deploycontext.SourceApp},
		"API_KEY": {deploycontext.MaskedValue, deploycontext.SourceContext},
	}
	for _, e := range eff.Env {
		if w, ok := want[e.Key]; ok && (e.Value != w[0] || e.Source != w[1]) {
			t.Errorf("%s = %+v, want %v", e.Key, e, w)
		}
	}

	if rec := ctxDo(t, rt, http.MethodGet, "/api/v1/apps/web/effective-config?context=nope", "", cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad context = %d", rec.Code)
	}
}

func TestApplyContextConfig(t *testing.T) {
	svc := spec.Service{
		Build: spec.Build{Type: spec.BuildStatic, Path: "public", Args: map[string]string{"KEEP": "1"}},
		Env:   map[string]spec.EnvVar{"A": {Value: "app"}, "S": {Secret: true}},
	}
	contexts := map[string]deploycontext.Config{
		deploycontext.DeployPreview:           {Env: map[string]string{"A": "ctx"}, BuildCommand: "make", Publish: "dist", BuildArgs: map[string]string{"N": "1"}},
		deploycontext.BranchContext("feat/*"): {Env: map[string]string{"A": "branch"}},
		deploycontext.BranchDeploy:            {Env: map[string]string{"A": "wrong-context"}},
	}
	applyContextConfig(&svc, deploycontext.DeployPreview, "feat/x", contexts)

	if svc.Env["A"].Value != "branch" || !svc.Env["S"].Secret {
		t.Fatalf("env = %+v", svc.Env)
	}
	if svc.Build.Command != "make" || svc.Build.Publish != "dist" || svc.Build.Path != "" || svc.Build.Args["KEEP"] != "1" || svc.Build.Args["N"] != "1" {
		t.Fatalf("build = %+v", svc.Build)
	}
}

func TestApplyContextConfig_BranchOverrideStillWins(t *testing.T) {
	svc := spec.Service{Env: map[string]spec.EnvVar{"A": {Value: "app"}}}
	applyContextConfig(&svc, deploycontext.BranchDeploy, "dev", map[string]deploycontext.Config{deploycontext.BranchDeploy: {Env: map[string]string{"A": "ctx"}}})
	overrides := []store.ServiceBranchEnvOverride{{BranchPattern: "dev", Key: "A", Value: "override"}}
	if err := applyBranchEnvOverrides(context.Background(), &svc, "web", "dev", overrides, nil); err != nil {
		t.Fatal(err)
	}
	if svc.Env["A"].Value != "override" {
		t.Fatalf("A = %q, want override", svc.Env["A"].Value)
	}
}
