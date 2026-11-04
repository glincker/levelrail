package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestStackSettingAudit(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	cookie := loginTestSession(t, rt, db)
	if code, out := rawCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks", `{"name":"base","scope":"install"}`)); code != http.StatusCreated {
		t.Fatalf("create install stack = %d %s", code, out)
	}
	doc := "version: 1\nkind: Setting\nmetadata:\n  name: " + runtimesettings.KeyHeartbeatJobAttempts + "\nspec:\n  value: 5\n"
	var pending, done stackApplyResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/base/apply", stackBody(t, doc, nil)), &pending); code != http.StatusAccepted {
		t.Fatalf("apply = %d %+v", code, pending)
	}
	second := storeUserWithAbilitiesForTest(t, db, "second-root@example.test", []string{AbilityRoot})
	if code := iacCall(t, rt, authedRequest(t, sessionCookieForTest(t, rt, second.ID), http.MethodPost, "/api/v1/stacks/base/revisions/1/approve", ""), &done); code != http.StatusOK {
		t.Fatalf("approve = %d %+v", code, done)
	}
	var found bool
	for _, m := range settingAuditDetails(t, db) {
		if m["key"] != runtimesettings.KeyHeartbeatJobAttempts {
			continue
		}
		found = true
		if m["stack"] != "base" || m["revision"] != float64(1) {
			t.Fatalf("audit row lacks the stack scope: %v", m)
		}
	}
	if !found {
		t.Fatal("no settings change audit row for the stack apply")
	}
}
