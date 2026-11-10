// Package e2e: live proof that a stored custom role, assigned through the
// real HTTP API, is denied a real action and the denial is audited. It runs
// the real router and store with the fake engine path only (stop flips
// desired state), so it is not a real host run.
package e2e

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eRolesAdminUsername = "e2e-roles-admin"
	e2eRolesAdminPassword = "e2e-roles-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func TestAccessRoles_Live_CustomRoleDeniedRealActionWithAudit(t *testing.T) {
	experimental.Set(experimental.AccessRoles)
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	b := &brand.Brand{Name: "Roles E2E Test Platform", BinaryName: "roles-e2e-test-platform"}
	server := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, db))
	const app = "roles-e2e-app"
	seedIAMTestApp(t, db, app)

	if err := api.BootstrapAdmin(context.Background(), db, e2eRolesAdminUsername, e2eRolesAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, server.URL, e2eRolesAdminUsername, e2eRolesAdminPassword)

	status, body := requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/roles",
		`{"name":"e2e-read-only","abilities":["read"],"visibility":"all"}`)
	if status != http.StatusCreated {
		t.Fatalf("create role: status = %d, want %d, body = %s", status, http.StatusCreated, body)
	}
	roleID := decodeField(t, body, "id")

	const email, password = "e2e-roles-viewer@example.com", "e2e-roles-viewer-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	status, body = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/auth/users",
		`{"email":"`+email+`","password":"`+password+`","role_id":"`+roleID+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create user with custom role: status = %d, want %d, body = %s", status, http.StatusCreated, body)
	}

	viewer := loginE2EClient(t, server.URL, email, password)
	if status, body = requestJSON(t, viewer, http.MethodGet, server.URL+"/api/v1/apps", ""); status != http.StatusOK {
		t.Fatalf("list apps as read-only role: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	status, body = requestJSON(t, viewer, http.MethodPost, server.URL+"/api/v1/apps/"+app+"/stop", "")
	if status != http.StatusForbidden {
		t.Fatalf("stop as read-only role: status = %d, want %d, body = %s", status, http.StatusForbidden, body)
	}
	svc, err := db.GetDesiredService(context.Background(), app)
	if err != nil {
		t.Fatalf("GetDesiredService(%q) error = %v", app, err)
	}
	if svc.Suspended {
		t.Errorf("Suspended = true, want false: the 403 must not change state")
	}

	status, body = requestJSON(t, admin, http.MethodGet, server.URL+"/api/v1/audit-log?limit=100", "")
	if status != http.StatusOK || !strings.Contains(string(body), "/apps/"+app+"/stop") || !strings.Contains(string(body), "403") {
		t.Fatalf("audit log: status = %d, want the denied stop entry, body = %.400s", status, body)
	}
}

// TestAccessRoles_Live_AbilitiesEditAndDeleteWhileAssigned covers abilities
// beyond read: a deploy role may stop, an edit reaches the holder at once, and
// a role that is still assigned or built in cannot be deleted.
func TestAccessRoles_Live_AbilitiesEditAndDeleteWhileAssigned(t *testing.T) {
	experimental.Set(experimental.AccessRoles)
	t.Cleanup(experimental.Reset)

	db := openLiveStore(t)
	b := &brand.Brand{Name: "Roles E2E Test Platform", BinaryName: "roles-e2e-test-platform"}
	server := newE2ETestServer(t, api.NewRouter(discardTestLogger(), b, db))
	const app = "roles-e2e-edit-app"
	seedIAMTestApp(t, db, app)
	if err := api.BootstrapAdmin(context.Background(), db, e2eRolesAdminUsername, e2eRolesAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	admin := loginE2EClient(t, server.URL, e2eRolesAdminUsername, e2eRolesAdminPassword)

	status, body := requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/roles",
		`{"name":"e2e-deployer","abilities":["read","deploy"],"visibility":"all"}`)
	if status != http.StatusCreated {
		t.Fatalf("create role: status = %d, body = %s", status, body)
	}
	roleID := decodeField(t, body, "id")
	const email, password = "e2e-roles-deployer@example.com", "e2e-roles-deployer-correct-horse" //nolint:gosec // test fixture credential, not a real secret
	status, body = requestJSON(t, admin, http.MethodPost, server.URL+"/api/v1/auth/users",
		`{"email":"`+email+`","password":"`+password+`","role_id":"`+roleID+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("create user: status = %d, body = %s", status, body)
	}
	user := loginE2EClient(t, server.URL, email, password)

	if status, body = requestJSON(t, user, http.MethodPost, server.URL+"/api/v1/apps/"+app+"/stop", ""); status >= 300 {
		t.Fatalf("stop with deploy ability: status = %d, body = %s", status, body)
	}
	if status, body = requestJSON(t, user, http.MethodPost, server.URL+"/api/v1/roles", `{"name":"e2e-escalate","abilities":["root"],"visibility":"all"}`); status != http.StatusForbidden {
		t.Errorf("role create without root: status = %d, want %d, body = %s", status, http.StatusForbidden, body)
	}

	if status, body = requestJSON(t, admin, http.MethodDelete, server.URL+"/api/v1/roles/"+roleID, ""); status != http.StatusConflict {
		t.Fatalf("delete assigned role: status = %d, want %d, body = %s", status, http.StatusConflict, body)
	}
	if status, body = requestJSON(t, admin, http.MethodDelete, server.URL+"/api/v1/roles/role_viewer", ""); status == http.StatusNoContent {
		t.Fatalf("delete built-in role succeeded, body = %s", body)
	}

	// Dropping deploy from the role must take effect for the holder without a new login.
	status, body = requestJSON(t, admin, http.MethodPut, server.URL+"/api/v1/roles/"+roleID,
		`{"name":"e2e-deployer","abilities":["read"],"visibility":"all"}`)
	if status != http.StatusOK {
		t.Fatalf("edit assigned role: status = %d, body = %s", status, body)
	}
	if status, body = requestJSON(t, user, http.MethodPost, server.URL+"/api/v1/apps/"+app+"/start", ""); status != http.StatusForbidden {
		t.Errorf("start after deploy removed: status = %d, want %d, body = %s", status, http.StatusForbidden, body)
	}
}

// TestAccessRoles_Live_MigrationKeepsExistingInstallData builds the schema from
// before roles existed, seeds a user, then upgrades and checks nothing is lost.
func TestAccessRoles_Live_MigrationKeepsExistingInstallData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")

	old, err := store.Open(ctx, path, store.WithMigrationCeiling(373))
	if err != nil {
		t.Fatalf("open pre-roles schema: %v", err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO users (id, email, display_name, abilities, created_at) VALUES (?, ?, ?, ?, ?)`,
		"u-old", "old@example.com", "Old", `["read","deploy"]`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed old-shape user: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close old db: %v", err)
	}

	db, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	roles, err := db.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	builtin := 0
	for _, r := range roles {
		if r.Builtin {
			builtin++
		}
	}
	if builtin != 4 {
		t.Errorf("built-in roles after upgrade = %d, want 4", builtin)
	}
	u, err := db.GetUserByID(ctx, "u-old")
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if strings.Join(u.Abilities, ",") != "read,deploy" {
		t.Errorf("abilities after upgrade = %v, want [read deploy]", u.Abilities)
	}
	if id, err := db.UserRoleID(ctx, "u-old"); err != nil || id != "" {
		t.Errorf("UserRoleID() = %q, %v, want no role label", id, err)
	}
}
