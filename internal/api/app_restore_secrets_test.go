package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newSecretsRestoreRouter(t *testing.T) (*Router, *store.DB, *secrets.Manager, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	m := secrets.NewManager(db, mk)
	rt := NewRouter(discardLogger(), testBrand(), db, WithSecretSetter(m))
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"SEC_A": "alpha-value", "SEC_B": "beta-value"} {
		if err := m.SetValue(ctx, "web", k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetLocked(ctx, "web", "SEC_B", true); err != nil {
		t.Fatal(err)
	}
	return rt, db, m, loginTestSession(t, rt, db)
}

func secretsDo(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec.Code
}

func requireNoSecrets(t *testing.T, db *store.DB, ns string) {
	t.Helper()
	keys, err := db.ListSecretKeys(context.Background(), ns)
	if err != nil || len(keys) != 0 {
		t.Fatalf("namespace %q keys = %v, %v, want none", ns, keys, err)
	}
	if _, err := db.GetServiceDEK(context.Background(), ns); !errors.Is(err, store.ErrServiceDEKNotFound) {
		t.Fatalf("namespace %q DEK err = %v, want not found", ns, err)
	}
}

func TestSoftDeleteApp_UndoRestoresSecretValues(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	requireNoSecrets(t, db, "web")
	if v, err := m.Resolve(ctx, "web", "SEC_A"); err == nil {
		t.Fatalf("live namespace still readable: %q", v)
	}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusOK {
		t.Fatalf("restore = %d", got)
	}
	for k, want := range map[string]string{"SEC_A": "alpha-value", "SEC_B": "beta-value"} {
		if got, err := m.Resolve(ctx, "web", k); err != nil || got != want {
			t.Fatalf("%s = %q, %v, want %q", k, got, err, want)
		}
	}
	if _, locked, _ := db.GetSecretKeyLocked(ctx, "web", "SEC_B"); !locked {
		t.Fatal("locked flag lost on restore")
	}
	requireNoSecrets(t, db, store.DeletedAppSecretsKey("web"))
}

func TestSoftDeleteApp_PurgeRemovesRetainedSecrets(t *testing.T) {
	t.Parallel()
	rt, db, _, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	ns := store.DeletedAppSecretsKey("web")
	if keys, _ := db.ListSecretKeys(ctx, ns); len(keys) != 2 {
		t.Fatalf("retained keys = %d, want 2", len(keys))
	}
	if n, err := rt.SweepRecentlyDeleted(ctx, time.Now().Add(400*24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}
	requireNoSecrets(t, db, ns)
	requireNoSecrets(t, db, "web")
}

func TestSoftDeleteApp_SameNameRecreateDoesNotSeeRetainedSecrets(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	// The name is held for the window; even a direct row write sees nothing under it.
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:2", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if keys, _ := m.ListKeys(ctx, "web"); len(keys) != 0 {
		t.Fatalf("recreated app lists %d secrets, want 0", len(keys))
	}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusConflict {
		t.Fatalf("restore over a live app = %d, want 409", got)
	}
	if keys, _ := m.ListKeys(ctx, "web"); len(keys) != 0 {
		t.Fatalf("refused restore leaked %d secrets into the live app", len(keys))
	}
	if keys, _ := db.ListSecretKeys(ctx, store.DeletedAppSecretsKey("web")); len(keys) != 2 {
		t.Fatalf("retained keys = %d, want 2 kept after refusal", len(keys))
	}
}

func TestHardDeleteApp_RemovesSecretsImmediately(t *testing.T) {
	t.Parallel()
	rt, db, _, cookie := newSecretsRestoreRouter(t)
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web?delete_data=true&confirm=web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	requireNoSecrets(t, db, "web")
	requireNoSecrets(t, db, store.DeletedAppSecretsKey("web"))
}

// failSuspendStore fails the last restore step while everything else hits the real store.
type failSuspendStore struct{ *store.DB }

func (failSuspendStore) UpdateServiceSuspended(context.Context, string, bool) error {
	return errors.New("suspend failed")
}

func TestRestoreDeletedApp_FailureMidwayKeepsRetainedSecrets(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	rt.apps = failSuspendStore{db}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusInternalServerError {
		t.Fatalf("restore = %d, want 500", got)
	}
	if _, err := db.GetDesiredService(ctx, "web"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("app after failed restore err = %v, want deleted", err)
	}
	requireNoSecrets(t, db, "web")
	if keys, _ := db.ListSecretKeys(ctx, store.DeletedAppSecretsKey("web")); len(keys) != 2 {
		t.Fatalf("retained keys = %d, want 2", len(keys))
	}
	rt.apps = db
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusOK {
		t.Fatalf("retry restore = %d", got)
	}
	if got, err := m.Resolve(ctx, "web", "SEC_A"); err != nil || got != "alpha-value" {
		t.Fatalf("SEC_A after retry = %q, %v", got, err)
	}
}
