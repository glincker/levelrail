package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func bulkCall(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, bulkAppsResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/bulk", body))
	var resp bulkAppsResponse
	if w.Code == http.StatusMultiStatus {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v body=%s", err, w.Body.String())
		}
	}
	return w.Code, resp
}

func bulkStatuses(resp bulkAppsResponse) map[string]string {
	out := map[string]string{}
	for _, r := range resp.Results {
		out[r.Name] = r.Status
	}
	return out
}

func TestBulkApps_PerAppAuthorization(t *testing.T) {
	actions := []struct{ action, value, ability string }{
		{bulkActionRestart, "", AbilityDeploy},
		{bulkActionStop, "", AbilityDeploy},
		{bulkActionStart, "", AbilityDeploy},
		{bulkActionAddTag, "team:core", AbilityWrite},
		{bulkActionRemoveTag, "team:core", AbilityWrite},
	}
	for _, tc := range actions {
		t.Run(tc.action, func(t *testing.T) {
			rt, db := newTestRouter(t)
			bootstrapTestAdmin(t, db)
			seedScheduledTaskApp(t, db, "secret-app")
			seedScheduledTaskApp(t, db, "open-app")
			user := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})
			attachTestPolicy(t, db, "deny-"+tc.action, "Deny", tc.ability, "app:secret-app", store.PrincipalTypeUser, user.ID)
			cookie := sessionCookieForTest(t, rt, user.ID)

			code, resp := bulkCall(t, rt, cookie, `{"action":"`+tc.action+`","names":["secret-app","open-app"],"value":"`+tc.value+`"}`)
			if code != http.StatusMultiStatus {
				t.Fatalf("status = %d, want 207", code)
			}
			got := bulkStatuses(resp)
			if got["secret-app"] != bulkStatusDenied {
				t.Errorf("secret-app = %q, want denied", got["secret-app"])
			}
			if got["open-app"] == bulkStatusDenied || got["open-app"] == bulkStatusError {
				t.Errorf("open-app = %q, want applied or skipped", got["open-app"])
			}
		})
	}
}

func TestBulkApps_DryRunChangesNothing(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	code, resp := bulkCall(t, rt, cookie, `{"action":"delete","names":["web"],"dry_run":true}`)
	if code != http.StatusMultiStatus || bulkStatuses(resp)["web"] != bulkStatusWouldRun {
		t.Fatalf("dry run = %d %+v", code, resp)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err != nil {
		t.Errorf("app should still exist: %v", err)
	}
}

func TestBulkApps_DeleteNeedsConfirmationAndAudits(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "a")
	seedScheduledTaskApp(t, db, "b")

	if code, _ := bulkCall(t, rt, cookie, `{"action":"delete","names":["a","b"]}`); code != http.StatusConflict {
		t.Fatalf("no confirmation status = %d, want 409", code)
	}
	if code, _ := bulkCall(t, rt, cookie, `{"action":"delete","names":["a","b"],"confirm_names":["a"]}`); code != http.StatusConflict {
		t.Fatalf("partial confirmation status = %d, want 409", code)
	}
	code, resp := bulkCall(t, rt, cookie, `{"action":"delete","names":["a","b"],"confirm_names":["b","a"]}`)
	if code != http.StatusMultiStatus || resp.Counts[bulkStatusOK] != 2 {
		t.Fatalf("confirmed delete = %d %+v", code, resp)
	}
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := 0
	for _, e := range entries {
		if e.Method == http.MethodDelete && (e.Path == "/api/v1/apps/a" || e.Path == "/api/v1/apps/b") {
			found++
		}
	}
	if found != 2 {
		t.Errorf("per-app delete audit rows = %d, want 2", found)
	}
}

func TestBulkApps_TagCapAndValidation(t *testing.T) {
	t.Setenv("APP_MAX_TAGS_PER_APP", "1")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	if code, _ := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"Bad Tag!"}`); code != http.StatusBadRequest {
		t.Errorf("invalid tag status = %d, want 400", code)
	}
	_, first := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"env:prod"}`)
	if bulkStatuses(first)["web"] != bulkStatusOK {
		t.Fatalf("first tag = %+v", first)
	}
	_, second := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"team:x"}`)
	if bulkStatuses(second)["web"] != bulkStatusSkipped || !strings.Contains(second.Results[0].Message, "maximum") {
		t.Errorf("over cap = %+v", second)
	}
	_, again := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"env:prod"}`)
	if bulkStatuses(again)["web"] != bulkStatusOK {
		t.Errorf("re-attaching existing tag = %+v, want ok", again)
	}
}

func TestBulkApps_SelectByTagAndMissingApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "worker")
	bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"tier:edge"}`)

	_, resp := bulkCall(t, rt, cookie, `{"action":"stop","tag":"tier:edge"}`)
	if got := bulkStatuses(resp); len(got) != 1 || got["web"] != bulkStatusOK {
		t.Errorf("tag selection = %+v", got)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"start","names":["ghost"]}`)
	if bulkStatuses(resp)["ghost"] != bulkStatusNotFound {
		t.Errorf("missing app = %+v", resp)
	}
}
