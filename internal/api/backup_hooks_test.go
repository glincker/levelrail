package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const hookTestSecret = "rcon-hunter2"

func TestVolumeBackupHooks_SensitiveGateAuditAndHiding(t *testing.T) {
	t.Parallel()
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	const path = "/api/v1/apps/web/volumes/data/backup-policy"

	abilities, _ := json.Marshal([]string{AbilityRead, AbilityWrite})
	code, tok := mintToken(t, rt, cookie, `{"name":"writer","abilities":`+string(abilities)+`}`)
	if code != http.StatusCreated {
		t.Fatalf("mint = %d", code)
	}
	bearer := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}
	if rec := bearer(http.MethodPut, `{"pre_hook":"evil"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("PUT without write:sensitive = %d, want 403", rec.Code)
	}

	if rec := do(t, rt, cookie, http.MethodPut, path, `{"pre_hook":"save-off `+hookTestSecret+`","post_hook":"save-on"}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	rec := bearer(http.MethodGet, "")
	var got volumeBackupPolicyResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.HooksHidden || got.PreHook != "" || strings.Contains(rec.Body.String(), hookTestSecret) {
		t.Fatalf("reader GET = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, rt, cookie, http.MethodGet, path, ""); !strings.Contains(rec.Body.String(), hookTestSecret) {
		t.Fatalf("owner GET hides hooks: %s", rec.Body.String())
	}

	if rec := do(t, rt, cookie, http.MethodPut, path, `{"post_hook":"save-on"}`); rec.Code != http.StatusOK {
		t.Fatalf("clear PUT = %d", rec.Code)
	}
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var details []string
	for _, e := range entries {
		if strings.Contains(e.Detail, hookTestSecret) || strings.Contains(e.Path, hookTestSecret) {
			t.Fatalf("audit leaked the hook command: %+v", e)
		}
		if strings.HasSuffix(e.Path, "#hooks") {
			details = append(details, e.Detail)
		}
	}
	joined := strings.Join(details, " ")
	if len(details) != 2 || !strings.Contains(joined, `"pre_hook":"set"`) || !strings.Contains(joined, `"pre_hook":"cleared"`) {
		t.Fatalf("hook audit rows = %v", details)
	}
}

// The hook visibility gate must follow the PUT route's resource decision,
// not the account's flat ability list.
func TestCallerMayEditBackupHooks_ResourceAware(t *testing.T) {
	t.Parallel()
	f := newTeamFixture(t)
	ctx := context.Background()
	full := []string{AbilityRead, AbilityWrite, AbilityWriteSensitive}
	member := func(role string) *http.Cookie {
		u, err := f.rt.createLocalUserInTeam(ctx, "hooks-"+role+"@team-a.test", "", teamMatrixPassword, full, teamA, role)
		if err != nil {
			t.Fatal(err)
		}
		return sessionCookieForTest(t, f.rt, u.ID)
	}
	may := func(c *http.Cookie, app string) bool {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/"+app+"/volumes/data/backup-policy", nil)
		req.AddCookie(c)
		return f.rt.callerMayEditBackupHooks(req, app)
	}
	viewer, developer, admin := member(store.TeamRoleViewer), member(store.TeamRoleDeveloper), member(store.TeamRoleAdmin)
	if !may(admin, appA) || !may(developer, appA) {
		t.Fatal("team admin and developer holding write:sensitive must see hooks of their app")
	}
	if may(viewer, appA) {
		t.Fatal("team viewer saw hooks")
	}
	if may(admin, appB) {
		t.Fatal("other team's hooks visible")
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.db.SaveEnvironment(ctx, store.Environment{ID: "env_hooks_prot", ProjectID: projA, Name: "protected", Protected: true, CreatedAt: "2026-10-01T00:00:00Z"}))
	must(f.db.SetServiceEnvironment(ctx, appA, "env_hooks_prot"))
	if may(developer, appA) {
		t.Fatal("non-admin saw hooks in a protected environment")
	}
	if !may(admin, appA) {
		t.Fatal("team admin lost hooks in a protected environment")
	}

	plain := storeUserWithAbilitiesForTest(t, f.db, "hooks-iam@example.com", full)
	plainCookie := sessionCookieForTest(t, f.rt, plain.ID)
	must(f.db.SavePolicy(ctx, store.Policy{ID: "pol_hooks_deny", Name: "deny-hooks",
		Document: `{"Statement":[{"Effect":"Deny","Action":["write:sensitive"],"Resource":["app:` + appA2 + `"]}]}`}))
	must(f.db.AttachPolicy(ctx, "att_hooks_deny", "pol_hooks_deny", store.PrincipalTypeUser, plain.ID))
	if may(plainCookie, appA2) {
		t.Fatal("IAM Deny on write:sensitive still showed hooks")
	}
}

func TestVolumeBackupHistory_HookOutputOnlyForSensitiveCallers(t *testing.T) {
	t.Parallel()
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	seedVolumeBackup(t, db, "bkh_hook", store.BackupStatusFailed)
	ctx := context.Background()
	if err := db.FinishBackupHistory(ctx, "bkh_hook", store.BackupStatusFailed, 0, "", "pre-backup hook failed: exit 3", "2026-10-01T00:00:01Z"); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveBackupHookOutput(ctx, "bkh_hook", "pre", "runtime-secret-xyz"); err != nil {
		t.Fatal(err)
	}
	const path = "/api/v1/apps/web/volumes/data/backups"
	if rec := do(t, rt, cookie, http.MethodGet, path, ""); !strings.Contains(rec.Body.String(), "runtime-secret-xyz") {
		t.Fatalf("sensitive caller lacks output: %s", rec.Body.String())
	}
	abilities, _ := json.Marshal([]string{AbilityRead})
	_, tok := mintToken(t, rt, cookie, `{"name":"reader","abilities":`+string(abilities)+`}`)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "runtime-secret-xyz") || !strings.Contains(rec.Body.String(), "pre-backup hook failed: exit 3") {
		t.Fatalf("reader = %d %s", rec.Code, rec.Body.String())
	}
}
