package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestTeamRoutes_AllowlistIsCurrent(t *testing.T) {
	registered := map[string]bool{}
	for _, r := range loadMatrixRoutes(t) {
		registered[r.key()] = true
	}
	for k := range tenantRoutes {
		if !registered[k] {
			t.Errorf("tenantRoutes entry %q is not a registered route, remove it", k)
		}
	}
	for k := range tenantSessionOnly {
		if !registered[k] {
			t.Errorf("tenantSessionOnly entry %q is not a registered route, remove it", k)
		}
	}
}

func createInvite(t *testing.T, f *teamFixture, asRole, body string) (int, teamInviteResource) {
	t.Helper()
	code, out := teamDo(t, f.rt, http.MethodPost, "/api/v1/teams/"+teamA+"/invites", body, withCookie(f.cookies[asRole]))
	var inv teamInviteResource
	_ = json.Unmarshal([]byte(out), &inv)
	return code, inv
}

func TestTeamInvites_RoleRules(t *testing.T) {
	f := newTeamFixture(t)
	tests := []struct {
		asRole, body string
		want         int
	}{
		{store.TeamRoleOwner, `{"role":"admin","email":"new-admin@x.test"}`, http.StatusCreated},
		{store.TeamRoleOwner, `{"role":"owner"}`, http.StatusBadRequest},
		{store.TeamRoleAdmin, `{"role":"developer"}`, http.StatusCreated},
		{store.TeamRoleAdmin, `{"role":"admin"}`, http.StatusForbidden},
		{store.TeamRoleDeveloper, `{"role":"viewer"}`, http.StatusForbidden},
		{store.TeamRoleViewer, `{"role":"viewer"}`, http.StatusForbidden},
	}
	for _, tc := range tests {
		if code, _ := createInvite(t, f, tc.asRole, tc.body); code != tc.want {
			t.Errorf("%s invites %s = %d, want %d", tc.asRole, tc.body, code, tc.want)
		}
	}
}

func TestTeamInvites_AcceptOnceExpiryRevokeAndEmailBinding(t *testing.T) {
	f := newTeamFixture(t)
	_, link := createInvite(t, f, store.TeamRoleOwner, `{"role":"developer"}`)
	accept := `{"token":"` + link.Token + `","email":"newbie@x.test","password":"` + teamMatrixPassword + `"}`
	if code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/team-invites/accept", accept, nil); code != http.StatusOK {
		t.Fatalf("accept = %d %s", code, body)
	}
	u, err := f.db.GetUserByEmail(context.Background(), "newbie@x.test")
	if err != nil {
		t.Fatal(err)
	}
	roles, _ := f.db.UserTeamRoles(context.Background(), u.ID)
	if len(roles) != 1 || roles[teamA] != store.TeamRoleDeveloper {
		t.Errorf("new user teams = %v, want only team A as developer", roles)
	}
	replay := `{"token":"` + link.Token + `","email":"second@x.test","password":"` + teamMatrixPassword + `"}`
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/team-invites/accept", replay, nil); code != http.StatusBadRequest {
		t.Errorf("replay = %d, want 400", code)
	}
	if _, err := f.db.GetUserByEmail(context.Background(), "second@x.test"); err == nil {
		t.Error("a replayed invite created a second account")
	}

	_, revoked := createInvite(t, f, store.TeamRoleOwner, `{"role":"viewer"}`)
	if code, _ := teamDo(t, f.rt, http.MethodDelete, "/api/v1/teams/"+teamA+"/invites/"+revoked.ID, "", withCookie(f.cookies[store.TeamRoleOwner])); code != http.StatusNoContent {
		t.Fatalf("revoke = %d", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/team-invites/accept", `{"token":"`+revoked.Token+`","email":"r@x.test","password":"`+teamMatrixPassword+`"}`, nil); code != http.StatusBadRequest {
		t.Errorf("revoked invite accepted: %d", code)
	}

	_, expiring := createInvite(t, f, store.TeamRoleOwner, `{"role":"viewer"}`)
	if _, err := f.db.ExecContext(context.Background(), `UPDATE team_invites SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), expiring.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/team-invites/accept", `{"token":"`+expiring.Token+`","email":"e@x.test","password":"`+teamMatrixPassword+`"}`, nil); code != http.StatusBadRequest {
		t.Errorf("expired invite accepted: %d", code)
	}

	_, bound := createInvite(t, f, store.TeamRoleOwner, `{"role":"viewer","email":"only-me@x.test"}`)
	other := storeUserWithAbilitiesForTest(t, f.db, "someone-else@x.test", []string{AbilityRead})
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/team-invites/accept", `{"token":"`+bound.Token+`"}`, withCookie(sessionCookieForTest(t, f.rt, other.ID))); code != http.StatusBadRequest {
		t.Errorf("email bound invite accepted by another account: %d", code)
	}
}

func TestTeamMembers_LastOwnerAndTransfer(t *testing.T) {
	f := newTeamFixture(t)
	owner := f.members[store.TeamRoleOwner]
	ownerCookie := withCookie(f.cookies[store.TeamRoleOwner])
	base := "/api/v1/teams/" + teamA + "/members/"
	if code, _ := teamDo(t, f.rt, http.MethodDelete, base+"me", "", ownerCookie); code != http.StatusConflict {
		t.Errorf("last owner leave = %d, want 409", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPut, base+owner.ID, `{"role":"viewer"}`, ownerCookie); code != http.StatusConflict {
		t.Errorf("last owner demote = %d, want 409", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodDelete, base+owner.ID, "", withCookie(f.cookies[store.TeamRoleAdmin])); code != http.StatusForbidden {
		t.Errorf("admin removes owner = %d, want 403", code)
	}
	admin := f.members[store.TeamRoleAdmin]
	if code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/teams/"+teamA+"/transfer", `{"user_id":"`+admin.ID+`"}`, ownerCookie); code != http.StatusNoContent {
		t.Fatalf("transfer = %d %s", code, body)
	}
	roles, _ := f.db.UserTeamRoles(context.Background(), owner.ID)
	if roles[teamA] != store.TeamRoleAdmin {
		t.Errorf("old owner role = %q, want admin", roles[teamA])
	}
	if code, _ := teamDo(t, f.rt, http.MethodDelete, base+"me", "", ownerCookie); code != http.StatusNoContent {
		t.Errorf("former owner leave = %d, want 204", code)
	}
	code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/teams/"+teamA+"/events", "", withCookie(f.cookies[store.TeamRoleAdmin]))
	if code != http.StatusOK || !strings.Contains(body, "team.ownership_transferred") || !strings.Contains(body, "member.left") {
		t.Errorf("team events = %d %s", code, body)
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/teams/"+teamA+"/events", "", withCookie(f.cookies[store.TeamRoleViewer])); code != http.StatusForbidden {
		t.Errorf("viewer reads team audit = %d, want 403", code)
	}
}

func TestTeamDefault_ManagedByInstanceAdminsOnly(t *testing.T) {
	f := newTeamFixture(t)
	op := storeUserWithAbilitiesForTest(t, f.db, "op@instance.test", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityDeploy})
	cookie := withCookie(sessionCookieForTest(t, f.rt, op.ID))
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/teams/"+store.DefaultTeamID+"/invites", `{"role":"developer"}`, cookie); code != http.StatusForbidden {
		t.Errorf("non-root default team admin invites = %d, want 403", code)
	}
	admin := withCookie(loginTestSession(t, f.rt, f.db))
	if code, _ := teamDo(t, f.rt, http.MethodDelete, "/api/v1/teams/"+store.DefaultTeamID, "", admin); code != http.StatusConflict {
		t.Errorf("delete default team = %d, want 409", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodDelete, "/api/v1/organizations/"+store.DefaultTeamID, "", admin); code != http.StatusConflict {
		t.Errorf("delete default organization = %d, want 409", code)
	}
}
