package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSoftDeleteApp_ReservesNameKeepsRevisionsAndRestoresStopped(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddEnvRevision(ctx, "web", "me", []string{"A"}, nil, 10, 0); err != nil {
		t.Fatal(err)
	}
	do := func(method, target, body string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
		return rec.Code
	}
	if got := do(http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	if revs, _ := db.ListEnvRevisions(ctx, "web", 10); len(revs) != 1 {
		t.Fatalf("revisions after soft delete = %d, want 1 kept", len(revs))
	}
	if got := do(http.MethodPost, "/api/v1/apps", `{"name":"web","image":"nginx:1","port":80}`); got != http.StatusConflict {
		t.Fatalf("same-name create in the window = %d, want 409", got)
	}
	restore := "/api/v1/recently-deleted/app/web/restore"
	if got := do(http.MethodPost, restore, `{"confirm":"other"}`); got != http.StatusBadRequest {
		t.Fatalf("restore with wrong confirm = %d, want 400", got)
	}
	if got := do(http.MethodPost, restore, `{"confirm":"web"}`); got != http.StatusOK {
		t.Fatalf("restore = %d", got)
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil || !svc.Suspended {
		t.Fatalf("restored app = %+v, %v, want stopped", svc, err)
	}
	if revs, _ := db.ListEnvRevisions(ctx, "web", 10); len(revs) != 1 {
		t.Fatalf("revisions after restore = %d, want 1", len(revs))
	}
}

func TestSoftDeleteApp_PurgeRemovesRevisions(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if _, err := db.AddEnvRevision(ctx, "old", "me", []string{"A"}, nil, 10, 0); err != nil {
		t.Fatal(err)
	}
	e := store.RecentlyDeleted{Kind: store.RecentlyDeletedApp, Name: "old", Snapshot: "{}", DeletedAt: time.Now().Add(-30 * 24 * time.Hour)}
	if err := db.AddRecentlyDeleted(ctx, e); err != nil {
		t.Fatal(err)
	}
	if n, err := rt.SweepRecentlyDeleted(ctx, time.Now()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	if revs, _ := db.ListEnvRevisions(ctx, "old", 10); len(revs) != 0 {
		t.Fatalf("revisions after purge = %d, want 0", len(revs))
	}
}

func TestRestoreDeletedApp_AuthzDeny(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	ctx := context.Background()
	snap := func(name, project string) string {
		b, _ := json.Marshal(store.DesiredService{Name: name, Image: "nginx:1", Port: 80, ProjectID: project})
		return string(b)
	}
	for _, e := range []store.RecentlyDeleted{
		{Kind: store.RecentlyDeletedApp, Name: appA, Snapshot: snap(appA, projA)},
		{Kind: store.RecentlyDeletedApp, Name: appB, Snapshot: snap(appB, projB)},
	} {
		if err := f.db.AddRecentlyDeleted(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.db.DeleteDesiredService(ctx, appA); err != nil {
		t.Fatal(err)
	}
	post := func(name string, mod func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/recently-deleted/app/"+name+"/restore", strings.NewReader(`{"confirm":"`+name+`"}`))
		mod(req)
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	dev := func(r *http.Request) { r.AddCookie(f.cookies[store.TeamRoleDeveloper]) }
	if got := post(appB, dev); got != http.StatusNotFound {
		t.Fatalf("other team restore = %d, want 404", got)
	}
	if got := post(appA, func(r *http.Request) { r.AddCookie(f.cookies[store.TeamRoleViewer]) }); got != http.StatusForbidden {
		t.Fatalf("viewer restore = %d, want 403", got)
	}
	const plaintext = "read-scoped-token" //nolint:gosec // test fixture
	if err := f.db.SaveAPIToken(ctx, store.APIToken{ID: "tok_read", Name: "reader", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if got := post(appA, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+plaintext) }); got != http.StatusForbidden {
		t.Fatalf("read token restore = %d, want 403", got)
	}
	if got := post(appA, dev); got != http.StatusOK {
		t.Fatalf("own team restore = %d, want 200", got)
	}
}

// failReserveStore fails the undo reservation write while every other call hits the real store.
type failReserveStore struct{ *store.DB }

func (failReserveStore) AddRecentlyDeleted(context.Context, store.RecentlyDeleted) error {
	return errors.New("reservation write failed")
}

func TestSoftDeleteApp_ReservationFailureAbortsDelete(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddEnvRevision(ctx, "web", "me", []string{"A"}, nil, 10, 0); err != nil {
		t.Fatal(err)
	}
	rt.apps = failReserveStore{db}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("delete = %d, want 500", rec.Code)
	}
	if _, err := db.GetDesiredService(ctx, "web"); err != nil {
		t.Fatalf("app removed without a reservation: %v", err)
	}
	if revs, _ := db.ListEnvRevisions(ctx, "web", 10); len(revs) != 1 {
		t.Fatalf("revisions = %d, want 1 kept with the app", len(revs))
	}
}

func TestRestoreDeletedApp_ProtectedEnvironmentNeedsAdmin(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	ctx := context.Background()
	if err := f.db.SaveEnvironment(ctx, store.Environment{ID: "env_prot_a", ProjectID: projA, Name: "production", Protected: true, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(store.DesiredService{Name: "prod-web", Image: "nginx:1", Port: 80, ProjectID: projA, EnvironmentID: "env_prot_a"})
	if err := f.db.AddRecentlyDeleted(ctx, store.RecentlyDeleted{Kind: store.RecentlyDeletedApp, Name: "prod-web", Snapshot: string(b)}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recently-deleted/app/prod-web/restore", strings.NewReader(`{"confirm":"prod-web"}`))
	req.AddCookie(f.cookies[store.TeamRoleDeveloper])
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("developer restore into protected env = %d, want 403", rec.Code)
	}
}

func TestRestoreDeletedDatabase_AuthzAndConfirm(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	ctx := context.Background()
	snap, _ := json.Marshal(store.DesiredDatabase{Name: "pg-a", Engine: "postgres", Version: "16"})
	if err := f.db.AddRecentlyDeleted(ctx, store.RecentlyDeleted{Kind: store.RecentlyDeletedDatabase, Name: "pg-a", Snapshot: string(snap)}); err != nil {
		t.Fatal(err)
	}
	post := func(body string, mod func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/recently-deleted/database/pg-a/restore", strings.NewReader(body))
		mod(req)
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for _, role := range []string{store.TeamRoleDeveloper, store.TeamRoleViewer} {
		if got := post(`{"confirm":"pg-a"}`, func(r *http.Request) { r.AddCookie(f.cookies[role]) }); got != http.StatusForbidden {
			t.Fatalf("%s team member restore = %d, want 403", role, got)
		}
	}
	if _, err := f.db.GetDesiredDatabase(ctx, "pg-a"); err == nil {
		t.Fatal("denied restore must not create the database")
	}
	if got := post(`{"confirm":"other"}`, func(r *http.Request) { r.AddCookie(loginTestSession(t, f.rt, f.db)) }); got != http.StatusBadRequest {
		t.Fatalf("wrong confirm = %d, want 400", got)
	}
	if got := post(`{}`, func(r *http.Request) { r.AddCookie(loginTestSession(t, f.rt, f.db)) }); got != http.StatusBadRequest {
		t.Fatalf("missing confirm = %d, want 400", got)
	}
	if _, err := f.db.GetDesiredDatabase(ctx, "pg-a"); err == nil {
		t.Fatal("unconfirmed restore must not create the database")
	}
}
