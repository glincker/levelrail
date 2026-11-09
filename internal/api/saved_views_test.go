package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type savedViewsList struct {
	Views []savedViewResource `json:"views"`
}

func listViews(t *testing.T, rt *Router, c *http.Cookie) []savedViewResource {
	t.Helper()
	code, body := teamDo(t, rt, http.MethodGet, "/api/v1/saved-views", "", func(r *http.Request) { r.AddCookie(c) })
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, body)
	}
	var out savedViewsList
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out.Views
}

func saveView(t *testing.T, rt *Router, c *http.Cookie, body string) (int, savedViewResource, string) {
	t.Helper()
	code, raw := teamDo(t, rt, http.MethodPost, "/api/v1/saved-views", body, func(r *http.Request) { r.AddCookie(c) })
	var v savedViewResource
	_ = json.Unmarshal([]byte(raw), &v)
	return code, v, raw
}

func viewNames(vs []savedViewResource) string {
	var n []string
	for _, v := range vs {
		n = append(n, v.Name)
	}
	return strings.Join(n, ",")
}

func newTeamBMember(t *testing.T, f *teamFixture, role string) *http.Cookie {
	t.Helper()
	u, err := f.rt.createLocalUserInTeam(context.Background(), role+"@team-b.test", "", teamMatrixPassword, store.TeamRoleAbilities(role), teamB, role)
	if err != nil {
		t.Fatal(err)
	}
	return sessionCookieForTest(t, f.rt, u.ID)
}

func TestSavedViewsScopedToOwnerAndTeam(t *testing.T) {
	f := newTeamFixture(t)
	dev, viewer, admin := f.cookies[store.TeamRoleDeveloper], f.cookies[store.TeamRoleViewer], f.cookies[store.TeamRoleAdmin]
	other := newTeamBMember(t, f, store.TeamRoleOwner)

	code, private, raw := saveView(t, f.rt, dev, `{"name":"failing api","list":"apps","filter":{"status":"failing","q":"api"}}`)
	if code != http.StatusCreated || private.Shared != store.SavedViewPrivate || private.Filter["status"] != "failing" || private.Filter["q"] != "api" {
		t.Fatalf("save private = %d %s", code, raw)
	}
	// A team_id in the body is ignored: the team comes from the session.
	code, shared, raw := saveView(t, f.rt, dev, `{"name":"pg","list":"databases","filter":{"engine":"postgres"},"shared":"team","team_id":"`+teamB+`"}`)
	if code != http.StatusCreated && code != http.StatusBadRequest {
		t.Fatalf("save shared = %d %s", code, raw)
	}
	if code == http.StatusBadRequest {
		if code, shared, raw = saveView(t, f.rt, dev, `{"name":"pg","list":"databases","filter":{"engine":"postgres"},"shared":"team"}`); code != http.StatusCreated {
			t.Fatalf("save shared = %d %s", code, raw)
		}
	}

	if got := viewNames(listViews(t, f.rt, dev)); got != "failing api,pg" {
		t.Fatalf("owner sees %q", got)
	}
	vv := listViews(t, f.rt, viewer)
	if len(vv) != 1 || vv[0].Name != "pg" || vv[0].Mine || vv[0].CanDelete {
		t.Fatalf("team member sees %+v, want only the shared view, read only", vv)
	}
	if got := listViews(t, f.rt, other); len(got) != 0 {
		t.Fatalf("other team sees %+v", got)
	}

	_, missing := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/sv_missing", "", func(r *http.Request) { r.AddCookie(other) })
	for _, id := range []string{private.ID, shared.ID} {
		c, body := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/"+id, "", func(r *http.Request) { r.AddCookie(other) })
		if c != http.StatusNotFound || stripRequestID(body) != stripRequestID(missing) {
			t.Fatalf("other team delete %s = %d %s, want the missing body %s", id, c, body, missing)
		}
	}
	if c, body := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/"+private.ID, "", func(r *http.Request) { r.AddCookie(admin) }); c != http.StatusNotFound {
		t.Fatalf("team admin delete of a private view = %d %s, want 404", c, body)
	}
	if c, body := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/"+shared.ID, "", func(r *http.Request) { r.AddCookie(viewer) }); c != http.StatusForbidden {
		t.Fatalf("viewer delete of a shared view = %d %s, want 403", c, body)
	}
	if c, body := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/"+shared.ID, "", func(r *http.Request) { r.AddCookie(admin) }); c != http.StatusNoContent {
		t.Fatalf("team admin delete = %d %s", c, body)
	}
	if c, body := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/"+private.ID, "", func(r *http.Request) { r.AddCookie(dev) }); c != http.StatusNoContent {
		t.Fatalf("owner delete = %d %s", c, body)
	}
	if got := listViews(t, f.rt, dev); len(got) != 0 {
		t.Fatalf("after delete %+v", got)
	}

	for _, action := range []string{AuditActionSavedViewCreate, AuditActionSavedViewShare, AuditActionSavedViewDelete} {
		var n int
		var detail string
		if err := f.db.QueryRowContext(context.Background(), `SELECT COUNT(*), COALESCE(MAX(detail), '') FROM audit_log WHERE action = ?`, action).Scan(&n, &detail); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("audit %s: no rows", action)
		}
		if strings.Contains(detail, `"filter"`) || strings.Contains(detail, "postgres") {
			t.Errorf("audit %s carries the filter: %q", action, detail)
		}
	}
}

func TestSavedViewsRejectHostileInput(t *testing.T) {
	f := newTeamFixture(t)
	dev := f.cookies[store.TeamRoleDeveloper]
	for name, body := range map[string]string{
		"unknown list":       `{"name":"x","list":"secrets","filter":{}}`,
		"key outside list":   `{"name":"x","list":"databases","filter":{"env":"prod"}}`,
		"redirect key":       `{"name":"x","list":"apps","filter":{"redirect":"https://evil.example"}}`,
		"proto key":          `{"name":"x","list":"apps","filter":{"__proto__":"1"}}`,
		"markup value":       `{"name":"x","list":"apps","filter":{"q":"<img src=x onerror=alert(1)>"}}`,
		"control value":      `{"name":"x","list":"apps","filter":{"q":"a\u0007b"}}`,
		"bidi value":         "{\"name\":\"x\",\"list\":\"apps\",\"filter\":{\"q\":\"a\\u202eb\"}}",
		"number value":       `{"name":"x","list":"apps","filter":{"q":1}}`,
		"nested value":       `{"name":"x","list":"apps","filter":{"q":{"a":"b"}}}`,
		"markup name":        `{"name":"<b>x</b>","list":"apps","filter":{}}`,
		"empty name":         `{"name":"  ","list":"apps","filter":{}}`,
		"long name":          `{"name":"` + strings.Repeat("n", 65) + `","list":"apps","filter":{}}`,
		"long value":         `{"name":"x","list":"apps","filter":{"q":"` + strings.Repeat("v", 513) + `"}}`,
		"unknown share mode": `{"name":"x","list":"apps","filter":{},"shared":"public"}`,
	} {
		if code, _, raw := saveView(t, f.rt, dev, body); code != http.StatusBadRequest {
			t.Errorf("%s = %d %s, want 400", name, code, raw)
		}
	}
	if got := listViews(t, f.rt, dev); len(got) != 0 {
		t.Fatalf("hostile input stored %+v", got)
	}
	if code, _, raw := saveView(t, f.rt, dev, `{"name":"ok","list":"apps","filter":{"q":"a b","tags":"web,api"}}`); code != http.StatusCreated {
		t.Fatalf("plain view = %d %s", code, raw)
	}
	if code, _, raw := saveView(t, f.rt, dev, `{"name":"ok","list":"apps","filter":{"q":"c"}}`); code != http.StatusOK {
		t.Fatalf("same name replaces = %d %s", code, raw)
	}
	if code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/saved-views?list=secrets", "", func(r *http.Request) { r.AddCookie(dev) }); code != http.StatusBadRequest {
		t.Fatalf("list filter = %d %s", code, body)
	}
}

func TestSavedViewsCapsFromSettings(t *testing.T) {
	t.Setenv("APP_SAVED_VIEW_MAX_PER_USER", "2")
	t.Setenv("APP_SAVED_VIEW_FILTER_MAX_KEYS", "2")
	f := newTeamFixture(t)
	dev := f.cookies[store.TeamRoleDeveloper]
	if code, _, raw := saveView(t, f.rt, dev, `{"name":"x","list":"apps","filter":{"q":"a","status":"failing","env":"prod"}}`); code != http.StatusBadRequest {
		t.Fatalf("three keys = %d %s, want 400", code, raw)
	}
	for _, n := range []string{"one", "two"} {
		if code, _, raw := saveView(t, f.rt, dev, `{"name":"`+n+`","list":"apps","filter":{"q":"a"}}`); code != http.StatusCreated {
			t.Fatalf("%s = %d %s", n, code, raw)
		}
	}
	if code, _, raw := saveView(t, f.rt, dev, `{"name":"three","list":"apps","filter":{}}`); code != http.StatusConflict {
		t.Fatalf("over the cap = %d %s, want 409", code, raw)
	}
	if code, _, raw := saveView(t, f.rt, dev, `{"name":"two","list":"apps","filter":{"q":"b"}}`); code != http.StatusOK {
		t.Fatalf("replacing at the cap = %d %s, want 200", code, raw)
	}
}

// A token limited to team A never reaches its owner's views in team B, not even by name.
func TestSavedViewsTeamLimitedTokenStaysInItsTeam(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	owner := f.members[store.TeamRoleOwner]
	if err := f.db.AddTeamMember(ctx, teamB, owner.ID, store.TeamRoleOwner); err != nil {
		t.Fatal(err)
	}
	inB := func(r *http.Request) {
		r.AddCookie(f.cookies[store.TeamRoleOwner])
		r.Header.Set(scopeTeamHeader, teamB)
	}
	code, raw := teamDo(t, f.rt, http.MethodPost, "/api/v1/saved-views", `{"name":"b only","list":"apps","filter":{"q":"x"}}`, inB)
	var bView savedViewResource
	_ = json.Unmarshal([]byte(raw), &bView)
	if code != http.StatusCreated {
		t.Fatalf("save in team B = %d %s", code, raw)
	}
	plain := "views-scoped-token-a"
	if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: "tok-views-a", Name: "tok-views-a", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityWrite}, OwnerUserID: owner.ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetAPITokenScope(ctx, store.APITokenScope{TokenID: "tok-views-a", TeamID: teamA, Role: store.TeamRoleOwner}); err != nil {
		t.Fatal(err)
	}
	tok := bearer(plain)
	if _, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/saved-views", "", tok); strings.Contains(body, bView.ID) {
		t.Fatalf("team A token lists the team B view: %s", body)
	}
	_, missing := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/sv_missing", "", tok)
	if c, body := teamDo(t, f.rt, http.MethodDelete, "/api/v1/saved-views/"+bView.ID, "", tok); c != http.StatusNotFound || stripRequestID(body) != stripRequestID(missing) {
		t.Fatalf("team A token delete = %d %s", c, body)
	}
	if c, body := teamDo(t, f.rt, http.MethodPost, "/api/v1/saved-views", `{"name":"b only","list":"apps","filter":{"q":"overwritten"}}`, tok); c != http.StatusNotFound {
		t.Fatalf("team A token overwrite = %d %s, want 404", c, body)
	}
	if v, err := f.db.GetSavedView(ctx, bView.ID); err != nil || v.Filter != `{"q":"x"}` {
		t.Fatalf("team B view changed: %+v %v", v, err)
	}
}

func TestSavedViewsAuditUpdateAndShare(t *testing.T) {
	f := newTeamFixture(t)
	dev := f.cookies[store.TeamRoleDeveloper]
	count := func(action string) int {
		var n int
		if err := f.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, body := range []string{
		`{"name":"v","list":"apps","filter":{"q":"a"}}`,
		`{"name":"v","list":"apps","filter":{"q":"b"}}`,
		`{"name":"v","list":"apps","filter":{"q":"b"},"shared":"team"}`,
		`{"name":"v","list":"apps","filter":{"q":"b"},"shared":"private"}`,
	} {
		if code, _, raw := saveView(t, f.rt, dev, body); code != http.StatusCreated && code != http.StatusOK {
			t.Fatalf("save = %d %s", code, raw)
		}
	}
	if c, u, s := count(AuditActionSavedViewCreate), count(AuditActionSavedViewUpdate), count(AuditActionSavedViewShare); c != 1 || u != 3 || s != 2 {
		t.Fatalf("create %d update %d share %d, want 1 3 2", c, u, s)
	}
	var detail string
	if err := f.db.QueryRowContext(context.Background(), `SELECT detail FROM audit_log WHERE action = ? LIMIT 1`, AuditActionSavedViewShare).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, `"team_id":"`+teamA+`"`) || strings.Contains(detail, `"q"`) {
		t.Fatalf("share detail %q", detail)
	}
}
