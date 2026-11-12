package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func snapshotWithVault(t *testing.T, db *store.DB) {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	hashes := store.AppEnvHashes("web", svc.Env, nil)
	for k, h := range store.VaultEnvHashes("web", svc.VaultEnv) {
		hashes[k] = h
	}
	fields := store.AppliedFields(*svc)
	fields[store.AppliedFieldSecretKeys] = ""
	if err := db.SaveAppliedConfig(context.Background(), store.AppliedConfig{
		ServiceName: "web", Release: application.ContainerName("web", application.NameImage(*svc), svc.RestartNonce),
		EnvHashes: hashes, Fields: fields, AppliedAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPendingChangesSeeSecretReferenceAddChangeRemove(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	snapshotFor(t, db, nil)
	vaultPath := "/api/v1/apps/web/vault-env/API_KEY"
	if p := pending(t, rt, cookie); p.Pending {
		t.Fatalf("fresh app pending: %+v", p)
	}

	step := func(name, method, body string) {
		t.Helper()
		if code := tlJSON(t, rt, cookie, method, vaultPath, body, nil); code != http.StatusOK && code != http.StatusNoContent {
			t.Fatalf("%s = %d", name, code)
		}
		p := pending(t, rt, cookie)
		if !p.Pending {
			t.Fatalf("%s did not mark pending: %+v", name, p)
		}
		svc, _ := db.GetDesiredService(context.Background(), "web")
		if !svc.EnvDirty {
			t.Fatalf("%s did not set env_dirty", name)
		}
	}
	step("add", http.MethodPut, `{"path":"app/cfg","key":"k"}`)
	p := pending(t, rt, cookie)
	if len(p.Changes) != 1 || p.Changes[0].Kind != pendingKindSecret || len(p.Changes[0].Keys) != 1 || p.Changes[0].Keys[0] != "API_KEY" {
		t.Fatalf("add change = %+v", p.Changes)
	}

	// apply recreates the container: restart nonce moves and the latch clears.
	before, _ := db.GetDesiredService(context.Background(), "web")
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/apply-pending", "", nil); code != http.StatusAccepted {
		t.Fatalf("apply = %d", code)
	}
	after, _ := db.GetDesiredService(context.Background(), "web")
	if after.RestartNonce == before.RestartNonce || after.EnvDirty {
		t.Fatalf("apply did not recreate: nonce %s -> %s dirty=%v", before.RestartNonce, after.RestartNonce, after.EnvDirty)
	}

	snapshotWithVault(t, db)
	step("change", http.MethodPut, `{"path":"app/cfg","key":"other"}`)
	apply := func() {
		if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/apply-pending", "", nil); code != http.StatusAccepted {
			t.Fatalf("apply = %d", code)
		}
	}
	apply()
	snapshotWithVault(t, db)
	step("remove", http.MethodDelete, "")
}

func TestVaultEnvRawRouteNeedsWriteAndRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	seedWebAppForTest(t, db)
	save := func(id, plain string, abilities ...string) {
		if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: id, TokenHash: hashToken(plain), Abilities: abilities, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	save("tok_rd", "read-deploy-token", AbilityRead, AbilityDeploy)
	save("tok_rw", "read-write-token", AbilityRead, AbilityWrite)
	do := func(token, body string) int {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/web/vault-env/K", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for _, body := range []string{
		`{"conn":"bao","path":"app1","key":"k"}`,
		`{"conn":"bao","path":"../sys/x","key":"k"}`,
		`{"ref":"secret://bao/app1#k"}`,
	} {
		if got := do("read-deploy-token", body); got != http.StatusForbidden {
			t.Errorf("read+deploy %s = %d, want 403", body, got)
		}
	}
	// A write token still needs root for a connection reference.
	if got := do("read-write-token", `{"conn":"bao","path":"app1","key":"k"}`); got != http.StatusForbidden {
		t.Errorf("read+write conn form = %d, want 403", got)
	}
}
