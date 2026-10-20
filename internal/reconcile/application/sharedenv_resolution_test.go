package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/sharedenv"
	"github.com/GLINCKER/levelrail/internal/store"
)

// TestController_ResolveEnv_SharedEnvVars proves that an app deployed
// under a project/organization/environment actually receives that
// scope's shared env vars at container-create time, end to end through
// the real internal/store tables, internal/secrets envelope encryption,
// internal/sharedenv.Resolver, and this package's own resolveEnv, not a
// hand-written fake standing in for any of those layers. This is the
// resolution path CLAUDE.md's app spec section (4.9) and the shared
// env var task both call out: a service never declares a { from: ... }
// reference for these, it inherits them automatically as resolveEnv's
// base layers (see resolveEnv's own doc comment on precedence).
func TestController_ResolveEnv_SharedEnvVars(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	secretsManager := secrets.NewManager(db, mk)

	if err := db.SaveOrganization(ctx, store.Organization{ID: "org_1", Name: "acme", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveOrganization() error = %v", err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SetProjectOrganization(ctx, "proj_1", "org_1"); err != nil {
		t.Fatalf("SetProjectOrganization() error = %v", err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_1", ProjectID: "proj_1", Name: "production", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveEnvironment() error = %v", err)
	}

	// Plain shared vars at all three tiers.
	if err := db.SetOrganizationEnvVars(ctx, "org_1", map[string]string{"ORG_VAR": "org-value"}); err != nil {
		t.Fatalf("SetOrganizationEnvVars() error = %v", err)
	}
	if err := db.SetProjectEnvVars(ctx, "proj_1", map[string]string{"PROJECT_VAR": "project-value"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}
	if err := db.SetEnvironmentEnvVars(ctx, "env_1", map[string]string{"ENV_VAR": "environment-value"}); err != nil {
		t.Fatalf("SetEnvironmentEnvVars() error = %v", err)
	}

	// A secret shared var at the project tier: encrypted via the real
	// secrets.Manager under store.ProjectEnvSecretsKey, exactly what PUT
	// /api/v1/projects/{id}/env/secrets/{key} does.
	if err := secretsManager.SetValue(ctx, store.ProjectEnvSecretsKey("proj_1"), "PROJECT_SECRET", "s3cr3t-value"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}
	if err := db.SetProjectSecretEnvVar(ctx, "proj_1", "PROJECT_SECRET"); err != nil {
		t.Fatalf("SetProjectSecretEnvVar() error = %v", err)
	}

	resolver := sharedenv.NewResolver(db, secretsManager)
	c := New("web", &fakeStore{}, newFakeRuntime(0),
		WithOrganizationEnv(resolver),
		WithProjectEnv(resolver),
		WithEnvironmentEnv(resolver),
	)

	desired := &store.DesiredService{
		Name:          "web",
		Image:         "img:v1",
		Port:          80,
		ProjectID:     "proj_1",
		EnvironmentID: "env_1",
		Env:           map[string]string{"APP_OWN": "app-value"},
	}

	got, err := c.resolveEnv(ctx, desired)
	if err != nil {
		t.Fatalf("resolveEnv() error = %v", err)
	}

	want := map[string]string{
		"ORG_VAR":        "org-value",
		"PROJECT_VAR":    "project-value",
		"ENV_VAR":        "environment-value",
		"PROJECT_SECRET": "s3cr3t-value",
		"APP_OWN":        "app-value",
	}
	if len(got) != len(want) {
		t.Errorf("resolveEnv() = %v, want %v", got, want)
	}
	for k, wantV := range want {
		if gotV := got[k]; gotV != wantV {
			t.Errorf("resolveEnv()[%q] = %q, want %q", k, gotV, wantV)
		}
	}
}

// TestController_ResolveEnv_SharedEnvVars_AppOwnOverridesSharedTiers
// pins resolveEnv's documented precedence (its own doc comment): the
// app's own Env always wins over a same-named shared var from any tier.
func TestController_ResolveEnv_SharedEnvVars_AppOwnOverridesSharedTiers(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.SetProjectEnvVars(ctx, "proj_1", map[string]string{"SHARED_KEY": "from-project"}); err != nil {
		t.Fatalf("SetProjectEnvVars() error = %v", err)
	}

	resolver := sharedenv.NewResolver(db, nil) // no master key configured
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithProjectEnv(resolver))

	desired := &store.DesiredService{
		Name:      "web",
		ProjectID: "proj_1",
		Env:       map[string]string{"SHARED_KEY": "app-override"},
	}

	got, err := c.resolveEnv(ctx, desired)
	if err != nil {
		t.Fatalf("resolveEnv() error = %v", err)
	}
	if got["SHARED_KEY"] != "app-override" {
		t.Errorf("resolveEnv()[SHARED_KEY] = %q, want the app's own value (app-override) to win over the project's shared value", got["SHARED_KEY"])
	}
}
