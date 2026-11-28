package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/secrets"
	"github.com/glincker/levelrail/internal/store"
)

const dbPassword = "db-password-value"

func newDatabaseSecretsRouter(t *testing.T) (*Router, *store.DB, *secrets.Manager, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	m := secrets.NewManager(db, mk)
	rt := NewRouter(discardLogger(), testBrand(), db, WithSecretSetter(m))
	cookie := loginTestSession(t, rt, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"orders","engine":"redis","version":"7"}`)
	if err := m.SetValue(context.Background(), "orders", "DB_PASSWORD", dbPassword); err != nil {
		t.Fatal(err)
	}
	return rt, db, m, cookie
}

func requireSecretRows(t *testing.T, db *store.DB, ns string, want int) {
	t.Helper()
	keys, err := db.ListSecretKeys(context.Background(), ns)
	if err != nil || len(keys) != want {
		t.Fatalf("namespace %q keys = %d, %v, want %d", ns, len(keys), err, want)
	}
}

func TestDatabaseDelete_SoftParksCredentialsAndNextAppSeesNothing(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newDatabaseSecretsRouter(t)
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/orders", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	requireNoSecrets(t, db, "orders")
	requireSecretRows(t, db, store.DeletedDatabaseSecretsKey("orders"), 1)
	// The name is held for the undo window, so an app cannot take it yet.
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"orders","image":"alpine:3.20","port":80}`); got == http.StatusCreated {
		t.Fatal("app created over a database name held for undo")
	}
	// After expiry the purge leaves no rows and the next app of the name sees nothing.
	if _, err := rt.SweepRecentlyDeleted(context.Background(), time.Now().Add(400*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	requireNoSecrets(t, db, store.DeletedDatabaseSecretsKey("orders"))
	requireNoSecrets(t, db, "orders")
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"orders","image":"alpine:3.20","port":80}`); got != http.StatusCreated {
		t.Fatalf("app create after expiry = %d", got)
	}
	if _, err := m.Resolve(context.Background(), "orders", "DB_PASSWORD"); err == nil {
		t.Fatal("new app of the same name resolved the deleted database password")
	}
}

func TestDatabaseDelete_UndoRestoresCredentials(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newDatabaseSecretsRouter(t)
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/orders", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/database/orders/restore", `{"confirm":"orders"}`); got != http.StatusOK {
		t.Fatalf("restore = %d", got)
	}
	if got, err := m.Resolve(context.Background(), "orders", "DB_PASSWORD"); err != nil || got != dbPassword {
		t.Fatalf("restored password = %q, %v", got, err)
	}
	requireNoSecrets(t, db, store.DeletedDatabaseSecretsKey("orders"))
}

func TestDatabaseDelete_PurgeLeavesNoRows(t *testing.T) {
	t.Parallel()
	rt, db, _, cookie := newDatabaseSecretsRouter(t)
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/orders?delete_data=true&confirm=orders", ""); got != http.StatusNoContent {
		t.Fatalf("purge delete = %d", got)
	}
	requireNoSecrets(t, db, "orders")
	requireNoSecrets(t, db, store.DeletedDatabaseSecretsKey("orders"))
}

func TestDatabaseDelete_CopyFailureAbortsAndKeepsCredentials(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newDatabaseSecretsRouter(t)
	rt.secrets = failCopySecrets{m}
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/databases/orders", ""); got == http.StatusNoContent {
		t.Fatal("delete succeeded despite the failed parking")
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "orders"); err != nil {
		t.Fatalf("database after failed delete: %v", err)
	}
	if got, err := m.Resolve(context.Background(), "orders", "DB_PASSWORD"); err != nil || got != dbPassword {
		t.Fatalf("live password = %q, %v", got, err)
	}
	requireNoSecrets(t, db, store.DeletedDatabaseSecretsKey("orders"))
}

// failNamespaceDelete fails erasing one namespace.
type failNamespaceDelete struct {
	*secrets.Manager
	ns string
}

func (f failNamespaceDelete) DeleteAll(ctx context.Context, ns string) error {
	if ns == f.ns {
		return errors.New("erase failed")
	}
	return f.Manager.DeleteAll(ctx, ns)
}

func TestDatabaseDelete_EraseFailureIsVisibleAndNextCreateWipes(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newDatabaseSecretsRouter(t)
	rt.secrets = failNamespaceDelete{m, "orders"}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/databases/orders?delete_data=true&confirm=orders", ""))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "credentials_pending") {
		t.Fatalf("delete = %d %s, want 202 credentials_pending", rec.Code, rec.Body.String())
	}
	requireSecretRows(t, db, "orders", 1)
	// The stale namespace is wiped before the name is reused.
	rt.secrets = m
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"orders","image":"alpine:3.20","port":80}`); got != http.StatusCreated {
		t.Fatalf("app create = %d", got)
	}
	requireNoSecrets(t, db, "orders")
}

func TestAppDelete_CredentialEraseFailureSurfaces(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	rt.secrets = failNamespaceDelete{m, "web"}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "erase stored secrets") {
		t.Fatalf("delete = %d %s, want 202 naming the failed erase", rec.Code, rec.Body.String())
	}
	if keys, _ := db.ListSecretKeys(context.Background(), "web"); len(keys) == 0 {
		t.Fatal("test setup: the failed erase should leave the values")
	}
}

func TestCreateDatabase_WipesStaleNamespaceAndRefusesControlPlane(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newDatabaseSecretsRouter(t)
	if err := m.SetValue(context.Background(), "ghost", "OLD", "stale-value"); err != nil {
		t.Fatal(err)
	}
	mustCreateDatabase(t, rt, cookie, `{"name":"ghost","engine":"redis","version":"7"}`)
	requireNoSecrets(t, db, "ghost")
	if err := rt.clearStaleSecretNamespace(context.Background(), "vault"); !errors.Is(err, errAppNameTaken) {
		t.Fatalf("control plane namespace err = %v, want refusal", err)
	}
}
