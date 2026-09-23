package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAppBranchEnvRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/branch-env"},
		{http.MethodPost, "/api/v1/apps/web/branch-env"},
		{http.MethodDelete, "/api/v1/apps/web/branch-env/benv_x"},
	})
}

func TestHandleListAppBranchEnv_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/branch-env", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleListAppBranchEnv_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/branch-env", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []branchEnvOverrideResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

func TestHandleSetAppBranchEnv_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	body := `{"branch_pattern":"release/*","key":"FEATURE_FLAG","value":"on"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got branchEnvOverrideResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID == "" || got.BranchPattern != "release/*" || got.Key != "FEATURE_FLAG" || got.Value != "on" || got.Secret {
		t.Errorf("response = %+v, want a non-empty id, pattern=release/* key=FEATURE_FLAG value=on secret=false", got)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 || overrides[0].Value != "on" {
		t.Fatalf("overrides = %+v, want 1 entry with value=on", overrides)
	}
}

func TestHandleSetAppBranchEnv_Upsert(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	first := httptest.NewRecorder()
	rt.Handler().ServeHTTP(first, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"main","key":"KEY","value":"v1"}`))
	var firstResp branchEnvOverrideResource
	if err := json.NewDecoder(first.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	second := httptest.NewRecorder()
	rt.Handler().ServeHTTP(second, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"main","key":"KEY","value":"v2"}`))
	var secondResp branchEnvOverrideResource
	if err := json.NewDecoder(second.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}

	if secondResp.ID != firstResp.ID {
		t.Errorf("second id = %q, want %q (same branch_pattern+key must upsert, not duplicate)", secondResp.ID, firstResp.ID)
	}
	overrides, err := db.ListServiceBranchEnvOverrides(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 || overrides[0].Value != "v2" {
		t.Fatalf("overrides = %+v, want 1 entry with value=v2", overrides)
	}
}

func TestHandleSetAppBranchEnv_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/missing/branch-env", `{"branch_pattern":"main","key":"KEY","value":"v1"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleSetAppBranchEnv_MissingFields(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	for _, body := range []string{
		`{"key":"KEY","value":"v1"}`,
		`{"branch_pattern":"main","value":"v1"}`,
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want %d", body, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleSetAppBranchEnv_InvalidPattern(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"[","key":"KEY","value":"v1"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppBranchEnv_SecretRequiresValue(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"main","key":"KEY","secret":true}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleSetAppBranchEnv_SecretNoSetterConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithSecretSetter
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"main","key":"KEY","value":"secret-value","secret":true}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleSetAppBranchEnv_SecretSuccess(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"main","key":"API_KEY","value":"sk-abc","secret":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got branchEnvOverrideResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Secret || got.Value != "" {
		t.Errorf("response = %+v, want secret=true and value never echoed back", got)
	}
	if setter.calls != 1 || setter.lastKey != "API_KEY" || setter.lastValue != "sk-abc" {
		t.Errorf("setter called with key=%q value=%q calls=%d, want key=API_KEY value=sk-abc calls=1", setter.lastKey, setter.lastValue, setter.calls)
	}

	overrides, err := db.ListServiceBranchEnvOverrides(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 || !overrides[0].Secret || overrides[0].Value != "" {
		t.Fatalf("overrides = %+v, want 1 secret entry with an empty stored value", overrides)
	}
}

func TestHandleDeleteAppBranchEnv_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	id, err := db.SetServiceBranchEnvOverride(context.Background(), "web", "main", "KEY", "value", false)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/branch-env/"+id, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	overrides, err := db.ListServiceBranchEnvOverrides(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides = %+v, want none after delete", overrides)
	}
}

func TestHandleDeleteAppBranchEnv_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web/branch-env/benv_doesnotexist", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleDeleteAppBranchEnv_WrongApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "other", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("seed other app: %v", err)
	}
	id, err := db.SetServiceBranchEnvOverride(context.Background(), "web", "main", "KEY", "value", false)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/other/branch-env/"+id, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s: an app must never delete another app's own override by id", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	overrides, err := db.ListServiceBranchEnvOverrides(context.Background(), "web")
	if err != nil {
		t.Fatalf("ListServiceBranchEnvOverrides() error = %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want web's own override to survive", overrides)
	}
}

// TestHandlePullRequestWebhook_AppliesBranchEnvOverride is the real,
// end-to-end proof for this feature: a branch-scoped override matching
// the PR's own head branch reaches the actual deploy request
// (deployPreviewSingle's applyBranchEnvOverrides call), winning over
// both the parent app's own env and the unscoped preview-env override,
// while a sibling key with no matching override still inherits from
// the parent untouched. githubPullRequestBody (preview_environments_test.go)
// always uses head ref "feature-x", so "feature-*" is this test's
// matching pattern and "release/*" its deliberately-non-matching one.
func TestHandlePullRequestWebhook_AppliesBranchEnvOverride(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Env: map[string]string{"FOO": "bar", "SHARED": "prod-value"},
	}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	unscoped := "unscoped-preview-value"
	if err := db.SetServicePreviewEnvOverride(context.Background(), "web", "SHARED", &unscoped); err != nil {
		t.Fatalf("SetServicePreviewEnvOverride() error = %v", err)
	}
	if _, err := db.SetServiceBranchEnvOverride(context.Background(), "web", "feature-*", "SHARED", "branch-value", false); err != nil {
		t.Fatalf("SetServiceBranchEnvOverride(feature-*) error = %v", err)
	}
	// Deliberately non-matching: proves a branch override never leaks
	// onto a preview built from a different branch.
	if _, err := db.SetServiceBranchEnvOverride(context.Background(), "web", "release/*", "FOO", "release-value", false); err != nil {
		t.Fatalf("SetServiceBranchEnvOverride(release/*) error = %v", err)
	}

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, secret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(builder.calls) != 1 {
		t.Fatalf("Deploy called %d times, want 1", len(builder.calls))
	}
	env := builder.calls[0].Service.Env
	if env["SHARED"].Value != "branch-value" {
		t.Errorf("Env[SHARED] = %+v, want Value = %q (branch override must win over both the parent's own env and the unscoped preview-env override)", env["SHARED"], "branch-value")
	}
	if env["FOO"].Value != "bar" {
		t.Errorf("Env[FOO] = %+v, want Value = %q (a non-matching branch pattern must never apply)", env["FOO"], "bar")
	}

	// The parent app's own env is untouched: an override is only ever
	// applied to a preview's deploy request, never to the parent itself.
	parent, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if parent.Env["SHARED"] != "prod-value" {
		t.Errorf("parent Env[SHARED] = %q, want %q untouched", parent.Env["SHARED"], "prod-value")
	}
}

// TestHandlePullRequestWebhook_AppliesSecretBranchEnvOverride proves the
// secret-marked case end to end, through a real secrets.Manager (not a
// fake): the plaintext set via POST .../branch-env is envelope-encrypted
// at rest and decrypted fresh by applyBranchEnvOverrides at preview
// deploy time, reaching the actual deploy request as a literal value.
func TestHandlePullRequestWebhook_AppliesSecretBranchEnvOverride(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(db, mk)
	gitSourceSecrets := newFakeGitSourceSecrets()
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(gitSourceSecrets), WithSecretSetter(manager))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	setPreviewEnabled(t, rt, cookie)
	builder := &sequencedBuilder{db: db, tag: "levelrail/web-pr-42:sha1"}
	rt.builder = builder
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, t.TempDir(), new(bool), nil)

	setRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(setRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/branch-env", `{"branch_pattern":"feature-*","key":"API_KEY","value":"sk-live-real","secret":true}`))
	if setRec.Code != http.StatusOK {
		t.Fatalf("set branch env: status = %d, body = %s", setRec.Code, setRec.Body.String())
	}

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	rec := sendPullRequestWebhook(rt, created.WebhookSecret, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if len(builder.calls) != 1 {
		t.Fatalf("Deploy called %d times, want 1", len(builder.calls))
	}
	env := builder.calls[0].Service.Env
	if env["API_KEY"].Value != "sk-live-real" {
		t.Errorf("Env[API_KEY] = %+v, want the decrypted plaintext %q", env["API_KEY"], "sk-live-real")
	}
	if env["API_KEY"].Secret {
		t.Errorf("Env[API_KEY].Secret = true, want false: deployPreviewSingle must resolve the secret to a literal, not defer it")
	}
}
