// Package e2e: live proof that a stored custom role, assigned through the
// real HTTP API, is denied a real action and the denial is audited. It runs
// the real router and store with the fake engine path only (stop flips
// desired state), so it is not a real host run.
package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
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
