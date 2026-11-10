package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	stackA       = "stack-own-a"
	stackMix     = "stack-mixed"
	approvalStkA = "appr_stack_a"
	approvalMix  = "appr_stack_mix"
)

type composeOwnerFixture struct {
	*teamFixture
	root, bAdmin, dualAdmin *http.Cookie
}

// newComposeOwnerFixture seeds a stack owned by team A and one split across A and B.
func newComposeOwnerFixture(t *testing.T) *composeOwnerFixture {
	t.Helper()
	f := &composeOwnerFixture{teamFixture: newTeamFixture(t)}
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for app, members := range map[string]map[string]string{
		stackA:   {stackA + "-web": projA, stackA + "-db": projA},
		stackMix: {stackMix + "-web": projA, stackMix + "-db": projB},
	} {
		must(f.db.SaveApp(ctx, store.App{ID: app, Name: app, CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z"}))
		for name, proj := range members {
			must(f.db.SaveDesiredService(ctx, store.DesiredService{Name: name, AppID: app, Image: "nginx:1", Port: 80}))
			must(f.db.UpdateServiceProject(ctx, name, proj))
		}
	}
	for id, app := range map[string]string{approvalStkA: stackA, approvalMix: stackMix} {
		must(f.db.SaveDeployApproval(ctx, store.DeployApproval{ID: id, ServiceName: app, Action: "deploy", Image: "nginx:2", Status: store.DeployApprovalStatusPending,
			CreatedAt: "2026-10-01T00:00:00Z", ExpiresAt: "2099-01-01T00:00:00Z"}))
	}
	f.root = loginTestSession(t, f.rt, f.db)
	bu, err := f.rt.createLocalUserInTeam(ctx, "admin@team-b.test", "", teamMatrixPassword, store.TeamRoleAbilities(store.TeamRoleAdmin), teamB, store.TeamRoleAdmin)
	must(err)
	f.bAdmin = sessionCookieForTest(t, f.rt, bu.ID)
	du, err := f.rt.createLocalUserInTeam(ctx, "dual@team-ab.test", "", teamMatrixPassword, store.TeamRoleAbilities(store.TeamRoleAdmin), teamA, store.TeamRoleAdmin)
	must(err)
	must(f.db.AddTeamMember(ctx, teamB, du.ID, store.TeamRoleAdmin))
	f.dualAdmin = sessionCookieForTest(t, f.rt, du.ID)
	return f
}

func (f *composeOwnerFixture) code(t *testing.T, method, path, body string, c *http.Cookie) int {
	t.Helper()
	code, _ := teamDo(t, f.rt, method, path, body, withCookie(c))
	return code
}

func TestComposeApprovalOwners_TeamOwningAllMembersDecides(t *testing.T) {
	f := newComposeOwnerFixture(t)
	get := "/api/v1/deploy-approvals/" + approvalStkA
	if got := f.code(t, http.MethodGet, get, "", f.bAdmin); got != http.StatusNotFound {
		t.Fatalf("team B admin GET = %d, want 404", got)
	}
	if got := f.code(t, http.MethodPost, get+"/reject", `{"reason":"no"}`, f.bAdmin); got != http.StatusNotFound {
		t.Fatalf("team B admin reject = %d, want 404", got)
	}
	if got := f.code(t, http.MethodPost, get+"/reject", `{"reason":"no"}`, f.cookies[store.TeamRoleViewer]); got != http.StatusForbidden {
		t.Fatalf("team A viewer reject = %d, want 403", got)
	}
	if got := f.code(t, http.MethodGet, get, "", f.cookies[store.TeamRoleViewer]); got != http.StatusOK {
		t.Fatalf("team A viewer GET = %d, want 200", got)
	}
	if got := f.code(t, http.MethodPost, get+"/reject", `{"reason":"no"}`, f.cookies[store.TeamRoleAdmin]); got != http.StatusOK {
		t.Fatalf("team A admin reject = %d, want 200", got)
	}
}

func TestComposeApprovalOwners_MixedTeamsNeedEveryTeam(t *testing.T) {
	f := newComposeOwnerFixture(t)
	path := "/api/v1/deploy-approvals/" + approvalMix
	for name, c := range map[string]*http.Cookie{"team A admin": f.cookies[store.TeamRoleAdmin], "team B admin": f.bAdmin} {
		if got := f.code(t, http.MethodGet, path, "", c); got != http.StatusNotFound {
			t.Errorf("%s GET = %d, want 404", name, got)
		}
		if got := f.code(t, http.MethodPost, path+"/reject", `{"reason":"no"}`, c); got != http.StatusNotFound {
			t.Errorf("%s reject = %d, want 404", name, got)
		}
	}
	if got := f.code(t, http.MethodPost, path+"/reject", `{"reason":"no"}`, f.dualAdmin); got != http.StatusOK {
		t.Fatalf("admin of both teams reject = %d, want 200", got)
	}
}

func TestComposeApprovalOwners_InstanceAdminDecidesMixed(t *testing.T) {
	f := newComposeOwnerFixture(t)
	if got := f.code(t, http.MethodPost, "/api/v1/deploy-approvals/"+approvalMix+"/reject", `{"reason":"no"}`, f.root); got != http.StatusOK {
		t.Fatalf("instance admin reject = %d, want 200", got)
	}
}

type failingAppGroups struct{ AppGroupLister }

func (failingAppGroups) ListServicesByApp(context.Context, string) ([]store.DesiredService, error) {
	return nil, errors.New("list members unavailable")
}

func TestComposeApprovalOwners_UnlistedMembersFailClosed(t *testing.T) {
	f := newComposeOwnerFixture(t)
	f.rt.appGroups = failingAppGroups{f.rt.appGroups}
	path := "/api/v1/deploy-approvals/" + approvalStkA
	if got := f.code(t, http.MethodGet, path, "", f.cookies[store.TeamRoleAdmin]); got != http.StatusNotFound {
		t.Fatalf("team A admin GET with unlisted members = %d, want 404", got)
	}
	if _, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/deploy-approvals?status=all", "", withCookie(f.cookies[store.TeamRoleAdmin])); strings.Contains(body, approvalStkA) {
		t.Fatalf("team A list shows a stack approval with unlisted members: %s", body)
	}
	if got := f.code(t, http.MethodGet, path, "", f.root); got != http.StatusOK {
		t.Fatalf("instance admin GET = %d, want 200", got)
	}
}

func TestComposeApprovalOwners_ListsAndFreeze(t *testing.T) {
	f := newComposeOwnerFixture(t)
	list := "/api/v1/deploy-approvals?status=all"
	_, a := teamDo(t, f.rt, http.MethodGet, list, "", withCookie(f.cookies[store.TeamRoleViewer]))
	if !strings.Contains(a, approvalStkA) || strings.Contains(a, approvalMix) || strings.Contains(a, approvalB) || !strings.Contains(a, approvalA) {
		t.Fatalf("team A list = %s", a)
	}
	_, b := teamDo(t, f.rt, http.MethodGet, list, "", withCookie(f.bAdmin))
	if strings.Contains(b, approvalStkA) || strings.Contains(b, approvalMix) || !strings.Contains(b, approvalB) {
		t.Fatalf("team B list = %s", b)
	}
	_, d := teamDo(t, f.rt, http.MethodGet, list, "", withCookie(f.dualAdmin))
	if !strings.Contains(d, approvalStkA) || !strings.Contains(d, approvalMix) {
		t.Fatalf("dual admin list = %s", d)
	}
	freeze := "/api/v1/apps/" + stackA + "/deploy-freeze"
	if got := f.code(t, http.MethodGet, freeze, "", f.bAdmin); got != http.StatusNotFound {
		t.Fatalf("team B freeze GET = %d, want 404", got)
	}
	// The fixture has no freeze store, so passing the gate answers 501.
	if got := f.code(t, http.MethodGet, freeze, "", f.cookies[store.TeamRoleViewer]); got == http.StatusNotFound || got == http.StatusForbidden {
		t.Fatalf("team A freeze GET = %d, want past the gate", got)
	}
}

func TestComposeApprovalOwners_SingleServiceUnchanged(t *testing.T) {
	f := newComposeOwnerFixture(t)
	if got := f.code(t, http.MethodGet, "/api/v1/deploy-approvals/"+approvalB, "", f.cookies[store.TeamRoleAdmin]); got != http.StatusNotFound {
		t.Fatalf("team A admin GET team B approval = %d, want 404", got)
	}
	if got := f.code(t, http.MethodPost, "/api/v1/deploy-approvals/"+approvalA+"/reject", `{"reason":"no"}`, f.cookies[store.TeamRoleDeveloper]); got != http.StatusForbidden {
		t.Fatalf("team A developer reject = %d, want 403", got)
	}
	if got := f.code(t, http.MethodPost, "/api/v1/deploy-approvals/"+approvalA+"/reject", `{"reason":"no"}`, f.cookies[store.TeamRoleAdmin]); got != http.StatusOK {
		t.Fatalf("team A admin reject = %d, want 200", got)
	}
}

func TestComposeApprovalOwners_StackNameCannotBeTaken(t *testing.T) {
	f := newComposeOwnerFixture(t)
	if code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps", `{"name":"`+stackA+`","image":"nginx:1","port":80,"project_id":"`+projB+`"}`, withCookie(f.bAdmin)); code != http.StatusConflict {
		t.Fatalf("team B create over stack name = %d %s, want 409", code, body)
	}
	if code, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/apps", `{"name":"fresh-b","image":"nginx:1","port":80,"project_id":"`+projB+`"}`, withCookie(f.bAdmin)); code != http.StatusCreated {
		t.Fatalf("team B ordinary create = %d %s, want 201", code, body)
	}
	path := "/api/v1/deploy-approvals/" + approvalStkA
	if got := f.code(t, http.MethodPost, path+"/reject", `{"reason":"no"}`, f.bAdmin); got != http.StatusNotFound {
		t.Fatalf("team B reject after create attempt = %d, want 404", got)
	}
	if got := f.code(t, http.MethodPost, path+"/reject", `{"reason":"no"}`, f.cookies[store.TeamRoleAdmin]); got != http.StatusOK {
		t.Fatalf("team A admin reject = %d, want 200", got)
	}
}

func (f *composeOwnerFixture) attach(t *testing.T, id, doc string, userID string) {
	t.Helper()
	ctx := context.Background()
	if err := f.db.SavePolicy(ctx, store.Policy{ID: id, Name: id, Document: doc}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AttachPolicy(ctx, "att_"+id, id, store.PrincipalTypeUser, userID); err != nil {
		t.Fatal(err)
	}
}

func TestComposeApprovalOwners_ListHonoursStackPolicies(t *testing.T) {
	f := newComposeOwnerFixture(t)
	f.attach(t, "pol_deny_stack", `{"Statement":[{"Effect":"Deny","Action":["read"],"Resource":["app:`+stackA+`"]}]}`, f.members[store.TeamRoleViewer].ID)
	f.attach(t, "pol_allow_stack", `{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:`+stackA+`"]}]}`, f.members[store.TeamRoleDeveloper].ID)
	list := "/api/v1/deploy-approvals?status=all"
	if _, body := teamDo(t, f.rt, http.MethodGet, list, "", withCookie(f.cookies[store.TeamRoleViewer])); strings.Contains(body, approvalStkA) {
		t.Fatalf("Deny on the stack still lists its approval: %s", body)
	}
	if _, body := teamDo(t, f.rt, http.MethodGet, list, "", withCookie(f.cookies[store.TeamRoleDeveloper])); !strings.Contains(body, approvalStkA) {
		t.Fatalf("Allow on the stack hides its approval: %s", body)
	}
}

func TestComposeApprovalOwners_MemberEnvironmentPolicyApplies(t *testing.T) {
	f := newComposeOwnerFixture(t)
	ctx := context.Background()
	for _, m := range []string{stackA + "-web", stackA + "-db"} {
		if err := f.db.SetServiceEnvironment(ctx, m, envA); err != nil {
			t.Fatal(err)
		}
	}
	f.attach(t, "pol_deny_env", `{"Statement":[{"Effect":"Deny","Action":["deploy"],"Resource":["`+resourcePrefixEnvironment+envA+`"]}]}`, f.members[store.TeamRoleAdmin].ID)
	compose := "/api/v1/apps/" + stackA + "/compose"
	if got := f.code(t, http.MethodPost, compose, `{}`, f.cookies[store.TeamRoleAdmin]); got != http.StatusForbidden {
		t.Fatalf("environment Deny on members, compose deploy = %d, want 403", got)
	}
	if got := f.code(t, http.MethodPost, compose, `{}`, f.cookies[store.TeamRoleOwner]); got == http.StatusForbidden || got == http.StatusNotFound {
		t.Fatalf("team A owner compose deploy = %d, want past the gate", got)
	}
}
