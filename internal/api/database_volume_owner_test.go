package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const keptVolume = "db-ledger-data"

type ownerFixture struct {
	f       *teamFixture
	vols    *fakeVolumeManager
	cookieA *http.Cookie
	cookieB *http.Cookie
	root    string
}

func newOwnerFixture(t *testing.T) *ownerFixture {
	t.Helper()
	f := newTeamFixture(t)
	f.rt.orphanedVolumes = newFakeVolumeManager()
	ub, err := f.rt.createLocalUserInTeam(context.Background(), "owner@team-b.test", "", teamMatrixPassword, store.TeamRoleAbilities(store.TeamRoleOwner), teamB, store.TeamRoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	return &ownerFixture{
		f: f, vols: f.rt.orphanedVolumes.(*fakeVolumeManager),
		cookieA: f.cookies[store.TeamRoleOwner], cookieB: sessionCookieForTest(t, f.rt, ub.ID),
		root: testAdminTokenFor(t, f.db),
	}
}

func (o *ownerFixture) post(t *testing.T, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	o.f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
	return rec
}

func (o *ownerFixture) createAndDeleteAsA(t *testing.T) {
	t.Helper()
	if rec := o.post(t, o.cookieA, `{"name":"ledger","engine":"redis","version":"7","project_id":"`+projA+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("team A create = %d %s", rec.Code, rec.Body.String())
	}
	o.vols.volumes[keptVolume] = true
	rec := httptest.NewRecorder()
	o.f.rt.Handler().ServeHTTP(rec, authedRequest(t, o.cookieA, http.MethodDelete, "/api/v1/databases/ledger", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("team A delete = %d %s", rec.Code, rec.Body.String())
	}
}

func bodyB(choice string) string {
	return `{"name":"ledger","engine":"redis","version":"7","project_id":"` + projB + `","existing_volume":"` + choice + `"}`
}

func TestLeftoverVolumeOtherTeamReuseRefusedDiscardAudited(t *testing.T) {
	t.Parallel()
	o := newOwnerFixture(t)
	o.createAndDeleteAsA(t)

	rec := o.post(t, o.cookieB, bodyB("reuse"))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), errCodeLeftoverOtherTeam) {
		t.Fatalf("team B reuse = %d %s", rec.Code, rec.Body.String())
	}
	for _, leak := range []string{teamA, projA} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("refusal leaks %q: %s", leak, rec.Body.String())
		}
	}
	if !o.vols.volumes[keptVolume] {
		t.Fatal("volume removed by a refused reuse")
	}

	if rec = o.post(t, o.cookieB, bodyB("discard")); rec.Code != http.StatusCreated {
		t.Fatalf("team B discard = %d %s", rec.Code, rec.Body.String())
	}
	if o.vols.volumes[keptVolume] {
		t.Fatal("discard did not wipe the kept volume")
	}
	rows, err := o.f.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range rows {
		if e.Method == auditLeftoverDiscard && strings.Contains(e.Path, "prev_team="+teamA) {
			found = true
		}
	}
	if !found {
		t.Fatal("no discard audit row naming the previous owner")
	}
	got, err := o.f.db.GetDatabaseVolumeOwner(context.Background(), "ledger")
	if err != nil || got == nil || got.TeamID != teamB {
		t.Fatalf("owner after discard = %+v, %v", got, err)
	}
}

func TestLeftoverVolumeSameTeamReuseAllowed(t *testing.T) {
	t.Parallel()
	o := newOwnerFixture(t)
	o.createAndDeleteAsA(t)
	body := `{"name":"ledger","engine":"redis","version":"7","project_id":"` + projA + `","existing_volume":"reuse"}`
	if rec := o.post(t, o.cookieA, body); rec.Code != http.StatusCreated {
		t.Fatalf("team A reuse = %d %s", rec.Code, rec.Body.String())
	}
	if !o.vols.volumes[keptVolume] {
		t.Fatal("own reuse removed the volume")
	}
}

func TestLeftoverVolumeUnknownOwnerNeedsRootConfirm(t *testing.T) {
	t.Parallel()
	o := newOwnerFixture(t)
	o.createAndDeleteAsA(t)
	if err := o.f.db.DeleteDatabaseVolumeOwner(context.Background(), "ledger"); err != nil {
		t.Fatal(err)
	}
	bodyA := `{"name":"ledger","engine":"redis","version":"7","project_id":"` + projA + `","existing_volume":"reuse"}`
	rec := o.post(t, o.cookieA, bodyA)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), errCodeLeftoverOwnerUnknown) {
		t.Fatalf("non-root reuse of unowned volume = %d %s", rec.Code, rec.Body.String())
	}

	rootPost := func(body string) *httptest.ResponseRecorder {
		return bindingsDo(t, o.f.rt, http.MethodPost, "/api/v1/databases", o.root, body)
	}
	rec = rootPost(`{"name":"ledger","engine":"redis","version":"7","existing_volume":"reuse"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), errCodeLeftoverConfirm) {
		t.Fatalf("root reuse without confirm = %d %s", rec.Code, rec.Body.String())
	}
	rec = rootPost(`{"name":"ledger","engine":"redis","version":"7","existing_volume":"reuse","confirm_volume":"ledger"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("root reuse with confirm = %d %s", rec.Code, rec.Body.String())
	}
}

func TestLeftoverVolumeUnknownOwnerNonRootDiscardWorks(t *testing.T) {
	t.Parallel()
	o := newOwnerFixture(t)
	o.createAndDeleteAsA(t)
	if err := o.f.db.DeleteDatabaseVolumeOwner(context.Background(), "ledger"); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"ledger","engine":"redis","version":"7","project_id":"` + projA + `","existing_volume":"discard"}`
	if rec := o.post(t, o.cookieA, body); rec.Code != http.StatusCreated {
		t.Fatalf("non-root discard = %d %s", rec.Code, rec.Body.String())
	}
	if o.vols.volumes[keptVolume] {
		t.Fatal("discard did not wipe the unowned volume")
	}
}
