package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func dbBulkCall(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, bulkDatabasesResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/bulk", body))
	var resp bulkDatabasesResponse
	if w.Code == http.StatusMultiStatus {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v body=%s", err, w.Body.String())
		}
	}
	return w.Code, resp
}

func dbBulkStatuses(resp bulkDatabasesResponse) map[string]string {
	out := map[string]string{}
	for _, r := range resp.Results {
		out[r.Name] = r.Status
	}
	return out
}

// dbBulkItemAudits counts per-database audit rows, leaving out the route row.
func dbBulkItemAudits(t *testing.T, db *store.DB) int {
	t.Helper()
	entries, err := db.ListAuditEntries(t.Context(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Path, "/api/v1/databases/") && e.Path != "/api/v1/databases/bulk" {
			n++
		}
	}
	return n
}

func seedSuspendedDatabase(t *testing.T, db *store.DB, name string) {
	t.Helper()
	seedDatabase(t, db, name)
	if err := db.UpdateDatabaseSuspended(t.Context(), name, true); err != nil {
		t.Fatal(err)
	}
}

func isSuspended(t *testing.T, db *store.DB, name string) bool {
	t.Helper()
	d, err := db.GetDesiredDatabase(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	return d.Suspended
}

func TestBulkDatabases_MixedResultsAuditEachItem(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedSuspendedDatabase(t, db, "stopped-db")
	seedDatabase(t, db, "running-db")

	code, resp := dbBulkCall(t, rt, cookie, `{"action":"start","names":["stopped-db","running-db","ghost-db"]}`)
	if code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207", code)
	}
	got := dbBulkStatuses(resp)
	want := map[string]string{"stopped-db": bulkStatusOK, "running-db": bulkStatusSkipped, "ghost-db": bulkStatusNotFound}
	for name, st := range want {
		if got[name] != st {
			t.Errorf("%s = %q, want %q", name, got[name], st)
		}
	}
	if isSuspended(t, db, "stopped-db") {
		t.Error("stopped-db should be started")
	}
	if n := dbBulkItemAudits(t, db); n != 3 {
		t.Errorf("per-item audit rows = %d, want 3", n)
	}

	_, resp = dbBulkCall(t, rt, cookie, `{"action":"backup","names":["running-db"]}`)
	if r := resp.Results; len(r) != 1 || r[0].Status != bulkStatusSkipped || r[0].Reason != dbSkipNoTarget {
		t.Errorf("backup without target = %+v, want skipped noTarget", r)
	}
}

func TestBulkDatabases_PerItemAuthorization(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	seedSuspendedDatabase(t, db, "secret-db")
	seedSuspendedDatabase(t, db, "open-db")
	user := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityWrite, AbilityWriteSensitive})
	attachTestPolicy(t, db, "deny-secret-db", "Deny", AbilityWriteSensitive, resourcePrefixDatabase+"secret-db", store.PrincipalTypeUser, user.ID)
	cookie := sessionCookieForTest(t, rt, user.ID)

	code, resp := dbBulkCall(t, rt, cookie, `{"action":"start","names":["secret-db","open-db"]}`)
	if code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207", code)
	}
	got := dbBulkStatuses(resp)
	if got["secret-db"] != bulkStatusDenied || got["open-db"] != bulkStatusOK {
		t.Fatalf("results = %v", got)
	}
	if !isSuspended(t, db, "secret-db") {
		t.Error("denied database must not change")
	}
}

func TestBulkDatabases_DryRunChangesNothing(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedDatabase(t, db, "web-db")
	seedSuspendedDatabase(t, db, "idle-db")

	code, resp := dbBulkCall(t, rt, cookie, `{"action":"stop","names":["web-db","idle-db"],"dry_run":true}`)
	if code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207", code)
	}
	got := dbBulkStatuses(resp)
	if got["web-db"] != dbBulkPlanned || got["idle-db"] != bulkStatusSkipped {
		t.Fatalf("plan = %v", got)
	}
	if isSuspended(t, db, "web-db") {
		t.Error("dry run stopped a database")
	}
	if n := dbBulkItemAudits(t, db); n != 0 {
		t.Errorf("dry run wrote %d per-item audit rows", n)
	}
}

func TestBulkDatabases_StopCountMismatchRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedDatabase(t, db, "a-db")
	seedDatabase(t, db, "b-db")

	for _, body := range []string{
		`{"action":"stop","names":["a-db","b-db"]}`,
		`{"action":"stop","names":["a-db","b-db"],"confirm_count":1}`,
	} {
		if code, _ := dbBulkCall(t, rt, cookie, body); code != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", body, code)
		}
	}
	if isSuspended(t, db, "a-db") || isSuspended(t, db, "b-db") {
		t.Error("refused stop changed a database")
	}
	if code, resp := dbBulkCall(t, rt, cookie, `{"action":"stop","names":["a-db","b-db"],"confirm_count":2}`); code != http.StatusMultiStatus || resp.Counts[bulkStatusOK] != 2 {
		t.Fatalf("confirmed stop = %d %+v", code, resp)
	}
}

func TestBulkDatabases_ProtectedEnvironmentRefusesStop(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := t.Context()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "shop", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", Protected: true, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	seedDatabase(t, db, "prod-db")
	seedDatabase(t, db, "dev-db")
	if err := db.SetDatabaseEnvironment(ctx, "prod-db", "env_prod"); err != nil {
		t.Fatal(err)
	}

	_, resp := dbBulkCall(t, rt, cookie, `{"action":"stop","names":["prod-db","dev-db"],"confirm_count":2}`)
	got := dbBulkStatuses(resp)
	if got["prod-db"] != dbBulkProtected || got["dev-db"] != bulkStatusOK {
		t.Fatalf("results = %v", got)
	}
	if isSuspended(t, db, "prod-db") {
		t.Error("protected database was stopped")
	}
}

// scopedBulkFixture gives a write:sensitive user a grant on one environment only.
func scopedBulkFixture(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	ctx := t.Context()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_vis", Name: "vis", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"env_vis_open", "env_vis_hidden"} {
		if err := db.SaveEnvironment(ctx, store.Environment{ID: id, ProjectID: "proj_vis", Name: id, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	for name, env := range map[string]string{"open-db": "env_vis_open", "hidden-db": "env_vis_hidden"} {
		seedDatabase(t, db, name)
		if err := db.SetDatabaseEnvironment(ctx, name, env); err != nil {
			t.Fatal(err)
		}
	}
	abilities := []string{AbilityRead, AbilityWrite, AbilityWriteSensitive}
	user := storeUserWithAbilitiesForTest(t, db, "scoped@example.com", abilities)
	if err := db.CreateRole(ctx, store.Role{ID: "role_vis", Name: "vis", Abilities: abilities, Visibility: store.RoleVisibilityGranted}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserRole(ctx, user.ID, "role_vis"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUserEnvironmentGrants(ctx, user.ID, []string{"env_vis_open"}); err != nil {
		t.Fatal(err)
	}
	return rt, db, sessionCookieForTest(t, rt, user.ID)
}

func TestBulkDatabases_HiddenAnswersLikeMissing(t *testing.T) {
	rt, db, cookie := scopedBulkFixture(t)
	code, resp := dbBulkCall(t, rt, cookie, `{"action":"stop","names":["hidden-db","open-db","ghost-db"],"confirm_count":3}`)
	if code != http.StatusMultiStatus {
		t.Fatalf("status = %d, want 207", code)
	}
	byName := map[string]bulkDatabaseResult{}
	for _, r := range resp.Results {
		byName[r.Name] = r
	}
	if byName["hidden-db"] != (bulkDatabaseResult{Name: "hidden-db", Status: bulkStatusNotFound, Message: "database not found"}) ||
		byName["ghost-db"] != (bulkDatabaseResult{Name: "ghost-db", Status: bulkStatusNotFound, Message: "database not found"}) {
		t.Errorf("hidden and missing differ: %+v", resp.Results)
	}
	if byName["open-db"].Status != bulkStatusOK || isSuspended(t, db, "hidden-db") {
		t.Errorf("results = %+v", resp.Results)
	}
}

func TestBulkDatabases_SelectorSkipsHiddenAndCounts(t *testing.T) {
	rt, _, cookie := scopedBulkFixture(t)
	code, resp := dbBulkCall(t, rt, cookie, `{"action":"stop","engine":"postgres","dry_run":true}`)
	if code != http.StatusMultiStatus || len(resp.Results) != 1 || resp.Results[0].Name != "open-db" {
		t.Fatalf("selector = %d %+v, want only open-db", code, resp.Results)
	}
	// The typed count is the visible count, so a mismatch message never reveals hidden ones.
	if code, _ := dbBulkCall(t, rt, cookie, `{"action":"stop","engine":"postgres","confirm_count":1}`); code != http.StatusMultiStatus {
		t.Errorf("confirmed visible stop = %d, want 207", code)
	}
}

func TestBulkDatabases_RejectsInvalidNames(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, body := range []string{`{"action":"start","names":["a/b"]}`, `{"action":"start","names":[""]}`} {
		if code, _ := dbBulkCall(t, rt, cookie, body); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, code)
		}
	}
	if n := dbBulkItemAudits(t, db); n != 0 {
		t.Errorf("rejected call wrote %d audit rows", n)
	}
}

func TestBulkDatabases_BackupQueuesThroughRunner(t *testing.T) {
	runner := newFakeBackupRunner()
	rt, db := newTestRouterWithBackupRunner(t, runner)
	cookie := loginTestSession(t, rt, db)
	ctx := t.Context()
	if err := db.SaveBackupTarget(ctx, store.BackupTarget{ID: "tgt_bulk", Name: "s3", Provider: "aws", Bucket: "backups"}); err != nil {
		t.Fatal(err)
	}
	seedDatabase(t, db, "pg-db")
	if err := db.SetDatabaseBackupSchedule(ctx, "pg-db", "tgt_bulk", "", 0, 0); err != nil {
		t.Fatal(err)
	}
	_, resp := dbBulkCall(t, rt, cookie, `{"action":"backup","names":["pg-db"]}`)
	if len(resp.Results) != 1 || resp.Results[0].Status != bulkStatusOK || resp.Results[0].BackupID == "" {
		t.Fatalf("results = %+v", resp.Results)
	}
	if c := runner.awaitCall(t); c.databaseName != "pg-db" || c.targetID != "tgt_bulk" || c.historyID != resp.Results[0].BackupID {
		t.Errorf("runner call = %+v", c)
	}
}
