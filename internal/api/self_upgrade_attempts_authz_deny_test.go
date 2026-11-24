package api

import (
	"net/http"
	"testing"
)

// The attempt actions can restore the pre-upgrade state, so they need root like starting an upgrade.
func TestAuthzDeny_SelfUpgradeAttemptActions(t *testing.T) {
	a := newAuthzDenyActors(t)
	for _, action := range []string{"fix", "retry", "cancel", "rollback"} {
		path := "/api/v1/updates/self-upgrade/attempts/x1/" + action
		for who, tok := range map[string]string{"developer token": a.devTok, "read token": a.readTok} {
			if rec := a.do(http.MethodPost, path, `{}`, nil, tok, ""); rec.Code != http.StatusForbidden {
				t.Errorf("%s as %s = %d, want 403", action, who, rec.Code)
			}
		}
		// Root passes the gate; this build has no upgrader wired so the handler answers 501.
		if rec := a.do(http.MethodPost, path, `{}`, nil, a.rootTok, ""); rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
			t.Errorf("%s as root token = %d, want the handler to answer", action, rec.Code)
		}
	}
}
