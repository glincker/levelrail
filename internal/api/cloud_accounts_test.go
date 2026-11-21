package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCloudAccounts_LegacyNodeProvidersStillConnectAndList(t *testing.T) {
	rt, cookie, _ := connectTestRouter(t, &fakeNodeProvisioner{})
	code, body := teamDo(t, rt, http.MethodPost, "/api/v1/node-providers", `{"provider":"hetzner","method":"api_token","token":"sekret","verify":true}`, withCookie(cookie))
	if code != http.StatusOK {
		t.Fatalf("legacy connect = %d %s", code, body)
	}
	code, body = teamDo(t, rt, http.MethodGet, "/api/v1/cloud-accounts", "", withCookie(cookie))
	var accts []cloudAccountWire
	if code != http.StatusOK || json.Unmarshal([]byte(body), &accts) != nil || len(accts) != 1 || accts[0].ID != "hetzner-default" || !accts[0].Linked {
		t.Fatalf("accounts = %d %s", code, body)
	}
	if strings.Contains(body, "sekret") {
		t.Error("account list echoed the credential")
	}
	if code, body = teamDo(t, rt, http.MethodGet, "/api/v1/node-providers/hetzner/regions", "", withCookie(cookie)); code != http.StatusOK {
		t.Errorf("legacy regions via default account = %d %s", code, body)
	}
	if code, body = teamDo(t, rt, http.MethodGet, "/api/v1/cloud-accounts/hetzner-default/regions", "", withCookie(cookie)); code != http.StatusOK {
		t.Errorf("account regions = %d %s", code, body)
	}
}

func TestCloudGrants_ViewCannotGrantOperateAndChangesAreAudited(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	root := loginTestSession(t, f.rt, f.db)
	if err := f.db.CreateCloudAccount(ctx, store.CloudAccount{ID: "oracle-prod", Provider: "oracle"}); err != nil {
		t.Fatal(err)
	}
	dev := f.members[store.TeamRoleDeveloper]
	grant := `{"principal_type":"user","principal_id":"` + dev.ID + `","actions":["view"]}`
	code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/cloud-accounts/oracle-prod/grants", grant, withCookie(root))
	if code != http.StatusCreated {
		t.Fatalf("root grant = %d %s", code, body)
	}
	var created cloudGrantWire
	_ = json.Unmarshal([]byte(body), &created)
	devCookie := withCookie(f.cookies[store.TeamRoleDeveloper])
	if code, _ = teamDo(t, f.rt, http.MethodGet, "/api/v1/cloud-accounts/oracle-prod", "", devCookie); code != http.StatusOK {
		t.Errorf("grantee view = %d", code)
	}
	escalate := `{"principal_type":"user","principal_id":"` + dev.ID + `","actions":["operate"]}`
	if code, _ = teamDo(t, f.rt, http.MethodPost, "/api/v1/cloud-accounts/oracle-prod/grants", escalate, devCookie); code != http.StatusForbidden {
		t.Errorf("view holder granting operate = %d, want 403", code)
	}
	if code, _ = teamDo(t, f.rt, http.MethodDelete, "/api/v1/cloud-accounts/oracle-prod/grants/"+created.ID, "", withCookie(root)); code != http.StatusNoContent {
		t.Fatalf("revoke = %d", code)
	}
	rows, err := f.db.ListAuditEntries(ctx, 100, nil, store.AuditEntryFilter{Action: "cloud.grant"})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range rows {
		seen[e.Action] = true
		if strings.Contains(e.Detail, "secret") {
			t.Errorf("audit detail leaks: %s", e.Detail)
		}
	}
	if !seen["cloud.grant.created"] || !seen["cloud.grant.deleted"] {
		t.Errorf("audit actions = %v", seen)
	}
}

func TestOrgIsolation_CloudAccounts(t *testing.T) {
	f := newOrgFixture(t)
	ctx := context.Background()
	if err := f.db.CreateCloudAccount(ctx, store.CloudAccount{ID: "acct-org-a", Provider: "hetzner", TeamID: teamA, DisplayName: "orga-cloud"}); err != nil {
		t.Fatal(err)
	}
	leaks := map[string]string{}
	checked := []string{"list", "detail", "action", "grants"}
	_, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/cloud-accounts", "", withCookie(f.bOwner))
	if strings.Contains(body, "acct-org-a") {
		leaks["list"] = "org B sees org A account: " + body
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/cloud-accounts/acct-org-a", "", withCookie(f.bOwner)); code == http.StatusOK {
		leaks["detail"] = "org B reads org A account"
	}
	if code, _ := teamDo(t, f.rt, http.MethodPost, "/api/v1/cloud-accounts/acct-org-a/verify", "", withCookie(f.bOwner)); code == http.StatusOK {
		leaks["action"] = "org B verifies org A account"
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/cloud-accounts/acct-org-a/grants", "", withCookie(f.bOwner)); code == http.StatusOK {
		leaks["grants"] = "org B lists org A grants"
	}
	if _, own := teamDo(t, f.rt, http.MethodGet, "/api/v1/cloud-accounts", "", withCookie(f.cookies[store.TeamRoleOwner])); !strings.Contains(own, "acct-org-a") {
		t.Errorf("org A owner does not see its own account: %s", own)
	}
	orgCases(t, "CloudAccounts", leaks, checked)
}
