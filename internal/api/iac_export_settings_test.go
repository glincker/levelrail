package api

import (
	"net/http"
	"testing"
)

func TestIaCExportSettingsNeedsRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	root := loginTestSession(t, rt, db)
	readOnly := storeUserWithAbilitiesForTest(t, db, "iac-settings@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, readOnly.ID)
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/export?settings=true", ""), nil); code != http.StatusForbidden {
		t.Fatalf("read-only settings export = %d, want 403", code)
	}
	if code := iacCall(t, rt, authedRequest(t, root, http.MethodGet, "/api/v1/export?settings=true", ""), nil); code != http.StatusOK {
		t.Fatalf("root settings export = %d", code)
	}
}
