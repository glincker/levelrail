package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type failCopySecrets struct{ *secrets.Manager }

func (failCopySecrets) CopyNamespace(context.Context, string, string) error {
	return errors.New("copy failed")
}

// failParkedDropSecrets fails erasing the parked namespace only.
type failParkedDropSecrets struct{ *secrets.Manager }

func (f failParkedDropSecrets) DeleteAll(ctx context.Context, ns string) error {
	if strings.HasPrefix(ns, "deleted-app/") {
		return errors.New("drop failed")
	}
	return f.Manager.DeleteAll(ctx, ns)
}

type failDeleteStore struct{ *store.DB }

func (failDeleteStore) DeleteDesiredService(context.Context, string) error {
	return errors.New("delete failed")
}

// failRollbackStore fails the last restore step and the rollback delete after it.
type failRollbackStore struct{ failSuspendStore }

func (failRollbackStore) DeleteDesiredService(context.Context, string) error {
	return errors.New("rollback delete failed")
}

func requireLiveSecrets(t *testing.T, m *secrets.Manager) {
	t.Helper()
	for k, want := range map[string]string{"SEC_A": "alpha-value", "SEC_B": "beta-value"} {
		if got, err := m.Resolve(context.Background(), "web", k); err != nil || got != want {
			t.Fatalf("live %s = %q, %v, want %q", k, got, err, want)
		}
	}
}

func TestSoftDelete_CopyNamespaceFailureKeepsAppAndSecrets(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	rt.secrets = failCopySecrets{m}
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got == http.StatusNoContent {
		t.Fatal("delete succeeded despite the failed parking")
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err != nil {
		t.Fatalf("app after failed delete: %v", err)
	}
	requireLiveSecrets(t, m)
}

func TestSoftDelete_RowDeleteFailureAfterParkingKeepsLiveSecrets(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	rt.apps = failDeleteStore{db}
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got == http.StatusNoContent {
		t.Fatal("delete succeeded despite the failed row delete")
	}
	requireLiveSecrets(t, m)
	requireNoSecrets(t, db, store.DeletedAppSecretsKey("web"))
}

func TestRestore_ParkedDropFailureStillRestoresLiveSecrets(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	rt.secrets = failParkedDropSecrets{m}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusOK {
		t.Fatalf("restore = %d, want 200", got)
	}
	requireLiveSecrets(t, m)
	if _, err := db.GetRecentlyDeleted(context.Background(), store.RecentlyDeletedApp, "web"); err != nil {
		t.Fatalf("undo entry must stay for the purge: %v", err)
	}
}

func TestRestore_RollbackDeleteFailureKeepsLiveSecretsAndParkedCopy(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	rt.apps = failRollbackStore{failSuspendStore{db}}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusInternalServerError {
		t.Fatalf("restore = %d, want 500", got)
	}
	if _, err := db.GetDesiredService(ctx, "web"); err != nil {
		t.Fatalf("app stays live after a failed rollback: %v", err)
	}
	requireLiveSecrets(t, m)
	// The purge always drops the parked copy, but a live name keeps its live secrets.
	if _, err := rt.SweepRecentlyDeleted(ctx, time.Now().Add(400*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	requireNoSecrets(t, db, store.DeletedAppSecretsKey("web"))
	requireLiveSecrets(t, m)
}

func TestRecentlyDeletedPurge_DropsParkedCopyAfterFailedDrop(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	rt.secrets = failParkedDropSecrets{m}
	if got := secretsDo(t, rt, cookie, http.MethodPost, "/api/v1/recently-deleted/app/web/restore", `{"confirm":"web"}`); got != http.StatusOK {
		t.Fatalf("restore = %d, want 200", got)
	}
	if keys, _ := db.ListSecretKeys(ctx, store.DeletedAppSecretsKey("web")); len(keys) != 2 {
		t.Fatalf("parked keys before purge = %d, want 2", len(keys))
	}
	if _, err := rt.SweepRecentlyDeleted(ctx, time.Now().Add(400*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	requireNoSecrets(t, db, store.DeletedAppSecretsKey("web"))
	requireLiveSecrets(t, m)
}

func TestRecentlyDeletedPurge_WaitsForNameLock(t *testing.T) {
	t.Parallel()
	rt, db, _, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	unlock := lockAppName("web")
	done := make(chan error, 1)
	go func() {
		_, err := rt.SweepRecentlyDeleted(ctx, time.Now().Add(400*24*time.Hour))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("purge ran while a restore held the name lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	if keys, _ := db.ListSecretKeys(ctx, store.DeletedAppSecretsKey("web")); len(keys) != 2 {
		t.Fatalf("parked keys during held lock = %d, want 2", len(keys))
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	requireNoSecrets(t, db, store.DeletedAppSecretsKey("web"))
}

func TestHardDeleteAndPurgeEraseTraceIngestToken(t *testing.T) {
	t.Parallel()
	rt, db, m, cookie := newSecretsRestoreRouter(t)
	ctx := context.Background()
	ns := store.TraceIngestSecretsKey("web")
	if err := m.SetValue(ctx, ns, "TOKEN", "t"); err != nil {
		t.Fatal(err)
	}
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web?delete_data=true&confirm=web", ""); got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	requireNoSecrets(t, db, ns)

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetValue(ctx, ns, "TOKEN", "t"); err != nil {
		t.Fatal(err)
	}
	if got := secretsDo(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web", ""); got != http.StatusNoContent {
		t.Fatalf("soft delete = %d", got)
	}
	if _, err := rt.SweepRecentlyDeleted(ctx, time.Now().Add(400*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	requireNoSecrets(t, db, ns)
}
