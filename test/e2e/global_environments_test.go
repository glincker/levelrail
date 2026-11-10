// Package e2e: live proof that a user whose custom role uses environment
// grants cannot see an app in another global environment, by list or by
// direct name. It runs the real router and store with no container engine,
// so it is not a real host run.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eEnvAdminUsername = "e2e-genv-admin"
	e2eEnvAdminPassword = "e2e-genv-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func TestGlobalEnvironments_Live_GrantedUserCannotSeeOtherEnvironment(t *testing.T) {
	experimental.Set(experimental.AccessRoles, experimental.GlobalEnvironments)
	t.Cleanup(experimental.Reset)

	ctx := context.Background()
	db := openLiveStore(t)
	b := &brand.Brand{Name: "Env E2E Test Platform", BinaryName: "env-e2e-test-platform"}
	server := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, db))
	const appA, appB = "genv-e2e-in-a", "genv-e2e-in-b"
	seedIAMTestApp(t, db, appA)
	seedIAMTestApp(t, db, appB)

	if err := api.BootstrapAdmin(ctx, db, e2eEnvAdminUsername, e2eEnvAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, server.URL, e2eEnvAdminUsername, e2eEnvAdminPassword)

	envIDs := map[string]string{}
	for _, name := range []string{"genv-a", "genv-b"} {
		status, body := requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/environments", `{"name":"`+name+`","kind":"custom"}`)
		if status != http.StatusCreated {
			t.Fatalf("create environment %q: status = %d, body = %s", name, status, body)
		}
		envIDs[name] = decodeField(t, body, "id")
	}
	if err := db.SetServiceEnvironment(ctx, appA, envIDs["genv-a"]); err != nil {
		t.Fatalf("tag %q: %v", appA, err)
	}
	if err := db.SetServiceEnvironment(ctx, appB, envIDs["genv-b"]); err != nil {
		t.Fatalf("tag %q: %v", appB, err)
	}

	const marker = "GENV_B_MARKER"
	if err := db.SetEnvironmentEnvVars(ctx, envIDs["genv-b"], map[string]string{marker: "value-b"}); err != nil {
		t.Fatalf("set env vars on B: %v", err)
	}

	status, body := requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/roles",
		`{"name":"e2e-env-guest","abilities":["read"],"visibility":"granted"}`)
	if status != http.StatusCreated {
		t.Fatalf("create role: status = %d, body = %s", status, body)
	}
	roleID := decodeField(t, body, "id")
	const email, password = "e2e-genv-guest@example.com", "e2e-genv-guest-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	status, body = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/auth/users",
		`{"email":"`+email+`","password":"`+password+`","role_id":"`+roleID+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create user: status = %d, body = %s", status, body)
	}
	userID := decodeField(t, body, "id")
	status, body = requestJSON(t, admin, http.MethodPut, server.URL+"/api/v1/users/"+userID+"/environment-grants",
		`{"environment_ids":["`+envIDs["genv-a"]+`"]}`)
	if status != http.StatusOK {
		t.Fatalf("grant environment A: status = %d, body = %s", status, body)
	}

	guest := loginE2EClient(t, server.URL, email, password)
	if status, body = requestJSON(t, guest, http.MethodGet, server.URL+"/api/v1/apps/"+appA, ""); status != http.StatusOK {
		t.Fatalf("granted app by id: status = %d, want 200, body = %s", status, body)
	}
	status, body = requestJSON(t, guest, http.MethodGet, server.URL+"/api/v1/apps", "")
	if status != http.StatusOK || !strings.Contains(string(body), appA) || strings.Contains(string(body), appB) {
		t.Fatalf("app list: status = %d, want only %q, body = %.400s", status, appA, body)
	}
	if status, body = requestJSON(t, guest, http.MethodGet, server.URL+"/api/v1/apps/"+appB, ""); status == http.StatusOK || strings.Contains(string(body), appB+"\"") {
		t.Fatalf("other environment app by direct name: status = %d, body = %.300s", status, body)
	}
	if status, body = requestJSON(t, guest, http.MethodGet, server.URL+"/api/v1/environments/"+envIDs["genv-b"]+"/env", ""); status == http.StatusOK || strings.Contains(string(body), marker) {
		t.Fatalf("other environment by direct id: status = %d, want a refusal, body = %.300s", status, body)
	}
}

// An install from before global environments keeps its project-scoped
// environment and the app tagged with it when the upgrade migration runs.
func TestGlobalEnvironments_Live_UpgradeKeepsExistingEnvironments(t *testing.T) {
	experimental.Set(experimental.GlobalEnvironments)
	t.Cleanup(experimental.Reset)

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	const preGlobalSchema = 374
	old, err := store.Open(ctx, path, store.WithMigrationCeiling(preGlobalSchema))
	if err != nil {
		t.Fatalf("open pre-upgrade store: %v", err)
	}
	for _, stmt := range []string{
		`INSERT INTO projects (id, name) VALUES ('proj_legacy', 'Legacy')`,
		`INSERT INTO environments (id, project_id, name) VALUES ('env_legacy', 'proj_legacy', 'Staging')`,
		`INSERT INTO environments (id, project_id, name) VALUES ('preview-env-legacy', 'proj_legacy', 'Preview')`,
		`INSERT INTO desired_services (name, image, port, updated_at, project_id, environment_id)
		 VALUES ('legacy-web', 'levelrail/genv-e2e:1', 8080, '2026-08-01T00:00:00Z', 'proj_legacy', 'env_legacy')`,
	} {
		if _, err := old.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed pre-upgrade row: %v", err)
		}
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close pre-upgrade store: %v", err)
	}

	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := api.BootstrapAdmin(ctx, db, e2eEnvAdminUsername, e2eEnvAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	b := &brand.Brand{Name: "Env E2E Test Platform", BinaryName: "env-e2e-test-platform"}
	server := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, db))
	admin := loginE2EClient(t, server.URL, e2eEnvAdminUsername, e2eEnvAdminPassword)

	status, body := requestJSON(t, admin, http.MethodGet, server.URL+"/api/v1/environments", "")
	if status != http.StatusOK {
		t.Fatalf("list environments: status = %d, body = %.300s", status, body)
	}
	var listed []struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		AppCount int    `json:"app_count"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode list %.300s: %v", body, err)
	}
	byID := map[string]int{}
	kinds := map[string]string{}
	for _, e := range listed {
		byID[e.ID] = e.AppCount
		kinds[e.ID] = e.Kind
	}
	for _, id := range []string{"env_dev", "env_test", "env_uat", "env_production"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("built-in environment %q missing after upgrade, body = %.400s", id, body)
		}
	}
	if n, ok := byID["env_legacy"]; !ok || n != 1 || kinds["env_legacy"] != "custom" {
		t.Errorf("legacy environment after upgrade: present=%v apps=%d kind=%q, body = %.400s", ok, n, kinds["env_legacy"], body)
	}
	if env, err := db.GetEnvironment(ctx, "preview-env-legacy"); err != nil || env.Kind != store.EnvironmentKindPreview {
		t.Errorf("legacy preview environment kind = %q, err = %v, want preview", env.Kind, err)
	}
}
