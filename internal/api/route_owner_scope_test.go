package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func denyTokenFor(t *testing.T, db *store.DB, ability, resource string) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveAPIToken(ctx, store.APIToken{ID: "ws", Name: "ws", TokenHash: hashToken("tok-ws"), Abilities: []string{ability}}); err != nil {
		t.Fatal(err)
	}
	doc := `{"Statement":[{"Effect":"Deny","Action":["` + ability + `"],"Resource":["` + resource + `"]}]}`
	if err := db.SavePolicy(ctx, store.Policy{ID: "pol_deny", Name: "deny", Document: doc}); err != nil {
		t.Fatal(err)
	}
	if err := db.AttachPolicy(ctx, "att_deny", "pol_deny", store.PrincipalTypeToken, "ws"); err != nil {
		t.Fatal(err)
	}
}

func TestGetBackupDrillHidesDeniedResource(t *testing.T) {
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	ctx := context.Background()
	seedVolumeBackup(t, db, "b", store.BackupStatusSucceeded)
	if err := db.StartBackupDrill(ctx, store.BackupDrill{ID: "bkd_1", BackupHistoryID: "b", ResourceKind: "volume", ServiceName: "web", VolumeName: "data", StartedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	denyTokenFor(t, db, AbilityRead, "app:web")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/backups/drills/bkd_1", "", "tok-ws"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("denied drill = %d %s, want 404", rec.Code, rec.Body)
	}
}

func TestProjectEnvSecretWriteHidesDeniedProject(t *testing.T) {
	setter := &fakeSecretSetter{}
	rt, db := newTestRouterWithSecrets(t, setter)
	if err := db.SaveProject(context.Background(), store.Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	denyTokenFor(t, db, AbilityWrite, "project:proj_1")
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, bearerRequest(method, "/api/v1/projects/proj_1/env/secrets/API_KEY", `{"value":"x"}`, "tok-ws"))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
			t.Fatalf("%s denied project = %d %s, want 403 or 404", method, rec.Code, rec.Body)
		}
	}
	if setter.calls != 0 {
		t.Fatalf("denied project secret was written: %d calls", setter.calls)
	}
}
