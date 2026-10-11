package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	plainSecretValue  = "plain-api-secret-value-0123456789"
	base64SecretValue = "ubahjctJ2K65pks5W9D7RDdT0ZYAax7hOtRPFO9Tfmc=" //nolint:gosec // test fixture shaped like the reported key
	storedSecretValue = "stored-secret-value-abcdefghijkl"
)

type envFixture struct {
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
}

func newEnvFixture(t *testing.T) envFixture {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithSecretSetter(secrets.NewManager(db, mk)))
	f := envFixture{rt: rt, db: db, cookie: loginTestSession(t, rt, db)}
	for _, name := range []string{"web", "other"} {
		err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "img:v1", Port: 3000, Env: map[string]string{
			"PORT": "3000", "API_SECRET": plainSecretValue, "QUARTER_V1": base64SecretValue,
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/secrets/DB_PASSWORD", `{"value":"`+storedSecretValue+`"}`, http.StatusNoContent)
	return f
}

func (f envFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, r)
	return rec
}

func (f envFixture) do(t *testing.T, cookie *http.Cookie, method, path, body string, want int) *httptest.ResponseRecorder {
	t.Helper()
	rec := f.serve(authedRequest(t, cookie, method, path, body))
	if rec.Code != want {
		t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func assertNoSecretValues(t *testing.T, surface, body string) {
	t.Helper()
	for _, v := range []string{plainSecretValue, base64SecretValue, storedSecretValue} {
		if strings.Contains(body, v) {
			t.Fatalf("%s leaks a sensitive value: %s", surface, body)
		}
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e codedError
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return e.Code
}

func TestEnvRedaction_EveryReadSurface(t *testing.T) {
	f := newEnvFixture(t)
	ctx := context.Background()
	if err := f.db.SetServicePreviewEnvOverride(ctx, "web", "API_SECRET", ptrTo(plainSecretValue)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SetServiceBranchEnvOverride(ctx, "web", "feat/*", "API_SECRET", plainSecretValue, false); err != nil {
		t.Fatal(err)
	}
	surfaces := []struct{ name, method, path, body string }{
		{"get app", http.MethodGet, "/api/v1/apps/web", ""},
		{"list apps", http.MethodGet, "/api/v1/apps", ""},
		{"masked export", http.MethodGet, "/api/v1/apps/web/env/export?format=dotenv", ""},
		{"masked json export", http.MethodGet, "/api/v1/apps/web/env/export?format=json", ""},
		{"keys export", http.MethodGet, "/api/v1/apps/web/env/export?values=none", ""},
		{"branch env", http.MethodGet, "/api/v1/apps/web/branch-env", ""},
		{"secrets list", http.MethodGet, "/api/v1/apps/web/secrets", ""},
		{"settings", http.MethodGet, "/api/v1/apps/web/env/settings", ""},
		{"audit log", http.MethodGet, "/api/v1/audit-log", ""},
		{"app events", http.MethodGet, "/api/v1/apps/web/changes", ""},
	}
	for _, s := range surfaces {
		rec := f.serve(authedRequest(t, f.cookie, s.method, s.path, s.body))
		if rec.Code >= 500 {
			t.Fatalf("%s: status %d", s.name, rec.Code)
		}
		assertNoSecretValues(t, s.name, rec.Body.String())
	}

	var app appResource
	if err := json.Unmarshal(f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web", "", http.StatusOK).Body.Bytes(), &app); err != nil {
		t.Fatal(err)
	}
	if app.Env["PORT"] != "3000" || app.EnvMeta["PORT"].Sensitive {
		t.Fatalf("PORT should stay visible: %+v", app.EnvMeta["PORT"])
	}
	for _, k := range []string{"API_SECRET", "QUARTER_V1"} {
		if app.Env[k] != envredact.Placeholder || !app.EnvMeta[k].ValueRedacted {
			t.Fatalf("%s not masked: %q", k, app.Env[k])
		}
	}
	if app.EnvMeta["QUARTER_V1"].Preview != "ubah...fmc=" {
		t.Fatalf("preview = %q", app.EnvMeta["QUARTER_V1"].Preview)
	}
}

func TestEnvRedaction_SharedLayersMaskAndKeep(t *testing.T) {
	f := newEnvFixture(t)
	ctx := context.Background()
	if err := f.db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p"}); err != nil {
		t.Fatal(err)
	}
	f.do(t, f.cookie, http.MethodPut, "/api/v1/projects/proj_1/env", `{"API_SECRET":"`+plainSecretValue+`","REGION":"eu"}`, http.StatusOK)
	got := f.do(t, f.cookie, http.MethodGet, "/api/v1/projects/proj_1/env", "", http.StatusOK).Body.String()
	assertNoSecretValues(t, "project env", got)
	got = f.do(t, f.cookie, http.MethodGet, "/api/v1/projects/proj_1/env/all", "", http.StatusOK).Body.String()
	assertNoSecretValues(t, "project env all", got)

	f.do(t, f.cookie, http.MethodPut, "/api/v1/projects/proj_1/env", `{"API_SECRET":"`+envredact.Placeholder+`","REGION":"us"}`, http.StatusOK)
	stored, err := f.db.ListProjectEnvVars(ctx, "proj_1")
	if err != nil || stored["API_SECRET"] != plainSecretValue || stored["REGION"] != "us" {
		t.Fatalf("placeholder round trip lost the value: %v %v", stored, err)
	}
	f.do(t, f.cookie, http.MethodPut, "/api/v1/projects/proj_1/env", `{"NEW_TOKEN":"`+envredact.Placeholder+`"}`, http.StatusBadRequest)
}

func TestEnvUpdate_PlaceholderRoundTrip(t *testing.T) {
	f := newEnvFixture(t)
	body := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web", "", http.StatusOK).Body.Bytes()
	var app appResource
	if err := json.Unmarshal(body, &app); err != nil {
		t.Fatal(err)
	}
	app.Env["PORT"] = "4000"
	raw, _ := json.Marshal(app)
	rec := f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", string(raw), http.StatusOK)
	assertNoSecretValues(t, "put response", rec.Body.String())
	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Env["API_SECRET"] != plainSecretValue || svc.Env["QUARTER_V1"] != base64SecretValue || svc.Env["PORT"] != "4000" {
		t.Fatalf("stored env after round trip = %v", svc.Env)
	}

	app.Env["BRAND_NEW_KEY"] = envredact.Placeholder
	raw, _ = json.Marshal(app)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", string(raw), http.StatusBadRequest)

	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps", `{"name":"fresh","image":"img:v1","port":80,"env":{"X":"`+envredact.Placeholder+`"}}`, http.StatusBadRequest)
}

func TestDeployCompare_MasksSnapshotValues(t *testing.T) {
	lit := store.DeployAttemptEnvKindLiteral
	from := []store.DeployAttemptEnvKey{{Key: "API_SECRET", Kind: lit, Value: plainSecretValue}, {Key: "PORT", Kind: lit, Value: "80"}}
	to := []store.DeployAttemptEnvKey{{Key: "API_SECRET", Kind: lit, Value: base64SecretValue}, {Key: "PORT", Kind: lit, Value: "81"}}
	raw, err := json.Marshal(struct {
		Changes []deployCompareEnvChange
		Side    deployCompareSide
	}{diffDeployCompareEnv(from, to), maskDeploySideEnv(deployCompareSide{Env: from})})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecretValues(t, "deploy compare", string(raw))
	if !strings.Contains(string(raw), `"81"`) {
		t.Fatalf("plain change lost: %s", raw)
	}
}
