package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestEnvironmentCompareRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/projects/proj_1/environments/compare?a=env_a&b=env_b"},
	})
}

// envCompareFixture seeds an org, a project under it, and two
// environments (env_a, env_b), each carrying shared env vars at all
// three tiers, secret-marked ones included: every
// TestHandleCompareEnvironmentEnv_* case below builds on this baseline.
// See each key's own name for which diff branch it exercises (ORG_/
// PROJ_ prefixes: inherited by both sides; ENV_: one side's own var;
// MIXED: secret on one side, plain on the other).
func envCompareFixture(t *testing.T, db *store.DB, manager *secrets.Manager) {
	t.Helper()
	ctx := context.Background()

	if err := db.SaveOrganization(ctx, store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-09-01T00:00:00Z", OrgID: "org_1"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.SetProjectOrganization(ctx, "proj_1", "org_1"); err != nil {
		t.Fatalf("set project organization: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_a", ProjectID: "proj_1", Name: "staging", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment a: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_b", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed environment b: %v", err)
	}

	if err := db.SetOrganizationEnvVars(ctx, "org_1", map[string]string{"ORG_PLAIN": "org-value"}); err != nil {
		t.Fatalf("set org env vars: %v", err)
	}
	if err := db.SetOrganizationSecretEnvVar(ctx, "org_1", "ORG_SECRET"); err != nil {
		t.Fatalf("mark org secret: %v", err)
	}
	if manager != nil {
		if err := manager.SetValueGuarded(ctx, store.OrganizationEnvSecretsKey("org_1"), "ORG_SECRET", "org-secret-plaintext", false); err != nil {
			t.Fatalf("seed org secret value: %v", err)
		}
	}

	if err := db.SetProjectEnvVars(ctx, "proj_1", map[string]string{"PROJ_PLAIN": "proj-value", "SHARED_KEY": "proj-shared"}); err != nil {
		t.Fatalf("set project env vars: %v", err)
	}
	if err := db.SetProjectSecretEnvVar(ctx, "proj_1", "PROJ_SECRET"); err != nil {
		t.Fatalf("mark project secret: %v", err)
	}
	if manager != nil {
		if err := manager.SetValueGuarded(ctx, store.ProjectEnvSecretsKey("proj_1"), "PROJ_SECRET", "proj-secret-plaintext", false); err != nil {
			t.Fatalf("seed project secret value: %v", err)
		}
	}

	if err := db.SetEnvironmentEnvVars(ctx, "env_a", map[string]string{"ENV_PLAIN_A": "a-value", "SHARED_KEY": "a-shared-override"}); err != nil {
		t.Fatalf("set env_a env vars: %v", err)
	}
	if err := db.SetEnvironmentSecretEnvVar(ctx, "env_a", "ENV_ONLY_SECRET"); err != nil {
		t.Fatalf("mark env_a secret: %v", err)
	}
	if err := db.SetEnvironmentSecretEnvVar(ctx, "env_a", "MIXED"); err != nil {
		t.Fatalf("mark env_a mixed secret: %v", err)
	}
	if manager != nil {
		if err := manager.SetValueGuarded(ctx, store.EnvironmentEnvSecretsKey("env_a"), "ENV_ONLY_SECRET", "env-a-secret-plaintext", false); err != nil {
			t.Fatalf("seed env_a secret value: %v", err)
		}
		if err := manager.SetValueGuarded(ctx, store.EnvironmentEnvSecretsKey("env_a"), "MIXED", "mixed-a-secret-plaintext", false); err != nil {
			t.Fatalf("seed env_a mixed secret value: %v", err)
		}
	}

	if err := db.SetEnvironmentEnvVars(ctx, "env_b", map[string]string{"ENV_PLAIN_B": "b-value", "MIXED": "mixed-b-plain-value"}); err != nil {
		t.Fatalf("set env_b env vars: %v", err)
	}
}

// secretPlaintexts lists every real secret plaintext envCompareFixture
// plants via the real internal/secrets.Manager: the exact set
// TestHandleCompareEnvironmentEnv_NeverLeaksSecretValues scans the raw
// response body for, substring by substring.
var secretPlaintexts = []string{
	"org-secret-plaintext",
	"proj-secret-plaintext",
	"env-a-secret-plaintext",
	"mixed-a-secret-plaintext",
}

// TestHandleCompareEnvironmentEnv_NeverLeaksSecretValues proves real,
// decryptable secret plaintext at every shared-env tier never reaches
// the wire, against the real internal/secrets.Manager, not a fake.
func TestHandleCompareEnvironmentEnv_NeverLeaksSecretValues(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey: %v", err)
	}
	manager := secrets.NewManager(db, mk)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(manager))
	cookie := loginTestSession(t, rt, db)
	envCompareFixture(t, db, manager)

	// Sanity check: every planted secret really is resolvable, so a test
	// that found no leak because the value was never actually stored
	// would be worthless.
	for namespace, key := range map[string]string{
		store.OrganizationEnvSecretsKey("org_1"): "ORG_SECRET",
		store.ProjectEnvSecretsKey("proj_1"):     "PROJ_SECRET",
		store.EnvironmentEnvSecretsKey("env_a"):  "ENV_ONLY_SECRET",
	} {
		if _, err := manager.Resolve(context.Background(), namespace, key); err != nil {
			t.Fatalf("sanity check: resolve %q/%q: %v", namespace, key, err)
		}
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_1/environments/compare?a=env_a&b=env_b", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := rec.Body.String()
	for _, plaintext := range secretPlaintexts {
		if strings.Contains(body, plaintext) {
			t.Errorf("response body leaks secret plaintext %q: %s", plaintext, body)
		}
	}

	var got environmentCompareResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	assertEnvEntry := func(side string, entries []environmentEnvEntryResource, key string, wantSecret bool, wantValue string) {
		for _, e := range entries {
			if e.Key != key {
				continue
			}
			if e.Secret != wantSecret {
				t.Errorf("%s side %q: secret = %v, want %v", side, key, e.Secret, wantSecret)
			}
			if e.Value != wantValue {
				t.Errorf("%s side %q: value = %q, want %q", side, key, e.Value, wantValue)
			}
			return
		}
		t.Errorf("%s side: key %q not found", side, key)
	}

	// Every secret-marked key, inherited or declared directly, carries no
	// value on either side, regardless of tier.
	assertEnvEntry("A", got.A.Env, "ORG_SECRET", true, "")
	assertEnvEntry("B", got.B.Env, "ORG_SECRET", true, "")
	assertEnvEntry("A", got.A.Env, "PROJ_SECRET", true, "")
	assertEnvEntry("B", got.B.Env, "PROJ_SECRET", true, "")
	assertEnvEntry("A", got.A.Env, "ENV_ONLY_SECRET", true, "")
	assertEnvEntry("A", got.A.Env, "MIXED", true, "")

	// Non-secret keys still show their real, resolved value: redaction
	// must not spill over onto plain vars.
	assertEnvEntry("A", got.A.Env, "ORG_PLAIN", false, "org-value")
	assertEnvEntry("B", got.B.Env, "ORG_PLAIN", false, "org-value")
	assertEnvEntry("A", got.A.Env, "PROJ_PLAIN", false, "proj-value")
	assertEnvEntry("A", got.A.Env, "SHARED_KEY", false, "a-shared-override")
	assertEnvEntry("B", got.B.Env, "SHARED_KEY", false, "proj-shared")
	assertEnvEntry("A", got.A.Env, "ENV_PLAIN_A", false, "a-value")
	assertEnvEntry("B", got.B.Env, "ENV_PLAIN_B", false, "b-value")
	// env_b never marked MIXED secret itself, so its own side shows the
	// real plain value it declared; only the cross-side diff (checked
	// below) has to stay conservative about it.
	assertEnvEntry("B", got.B.Env, "MIXED", false, "mixed-b-plain-value")

	diffByKey := make(map[string]environmentEnvDiffEntry, len(got.Diff))
	for _, d := range got.Diff {
		diffByKey[d.Key] = d
	}

	for _, plain := range []string{"ORG_PLAIN", "PROJ_PLAIN"} {
		if d, ok := diffByKey[plain]; ok {
			t.Errorf("identical plain key %q should not appear in diff, got %+v", plain, d)
		}
	}

	wantStatus := map[string]string{
		"ORG_SECRET":      envDiffMasked,
		"PROJ_SECRET":     envDiffMasked,
		"MIXED":           envDiffMasked,
		"SHARED_KEY":      envDiffChanged,
		"ENV_PLAIN_A":     envDiffOnlyInA,
		"ENV_PLAIN_B":     envDiffOnlyInB,
		"ENV_ONLY_SECRET": envDiffOnlyInA,
	}
	for key, want := range wantStatus {
		d, ok := diffByKey[key]
		if !ok {
			t.Errorf("diff missing key %q", key)
			continue
		}
		if d.Status != want {
			t.Errorf("diff[%q].Status = %q, want %q", key, d.Status, want)
		}
		if want == envDiffMasked || key == "ENV_ONLY_SECRET" {
			if d.A != "" || d.B != "" {
				t.Errorf("diff[%q] leaked a value: a=%q b=%q", key, d.A, d.B)
			}
		}
	}

	if d := diffByKey["SHARED_KEY"]; d.A != "a-shared-override" || d.B != "proj-shared" {
		t.Errorf("diff[SHARED_KEY] = %+v, want a=a-shared-override b=proj-shared", d)
	}
	if d := diffByKey["ENV_PLAIN_A"]; d.A != "a-value" {
		t.Errorf("diff[ENV_PLAIN_A].A = %q, want a-value", d.A)
	}
	if d := diffByKey["ENV_PLAIN_B"]; d.B != "b-value" {
		t.Errorf("diff[ENV_PLAIN_B].B = %q, want b-value", d.B)
	}
}

func TestHandleCompareEnvironmentEnv_MissingQueryParams(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	envCompareFixture(t, db, nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_1/environments/compare?a=env_a", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleCompareEnvironmentEnv_UnknownProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	envCompareFixture(t, db, nil)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_missing/environments/compare?a=env_a&b=env_b", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleCompareEnvironmentEnv_EnvironmentNotInProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	envCompareFixture(t, db, nil)
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_other", Name: "other", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed other project: %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_other", ProjectID: "proj_other", Name: "staging", CreatedAt: "2026-09-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed env_other: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_1/environments/compare?a=env_a&b=env_other", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
