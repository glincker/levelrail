package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCITrustCreateNeedsDeployAndRootForCustomIssuer(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	sensitiveOnly := seedMatrixToken(t, db, "tok-ws", []string{AbilityWriteSensitive})
	withDeploy := seedMatrixToken(t, db, "tok-wsd", []string{AbilityWriteSensitive, AbilityDeploy})
	post := func(auth func(*http.Request), body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/ci/trusts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		auth(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	session := func(r *http.Request) { r.AddCookie(cookie) }

	if got := post(bearer(sensitiveOnly), `{"repository":"acme/web","ref_pattern":"refs/heads/main"}`); got != http.StatusForbidden {
		t.Fatalf("write:sensitive without deploy = %d, want 403", got)
	}
	if got := post(bearer(withDeploy), `{"repository":"acme/web","ref_pattern":"refs/heads/main"}`); got != http.StatusCreated {
		t.Fatalf("write:sensitive plus deploy, github = %d, want 201", got)
	}
	if got := post(bearer(withDeploy), `{"provider":"gitlab","repository":"acme/web","ref_pattern":"refs/tags/v*"}`); got != http.StatusCreated {
		t.Fatalf("write:sensitive plus deploy, gitlab.com = %d, want 201", got)
	}
	custom := `{"provider":"gitlab","issuer":"https://gitlab.example.com","repository":"acme/web","ref_pattern":"refs/heads/release/*"}`
	if got := post(bearer(withDeploy), custom); got != http.StatusForbidden {
		t.Fatalf("custom issuer by a non-root caller = %d, want 403", got)
	}
	if got := post(session, custom); got != http.StatusCreated {
		t.Fatalf("custom issuer by root = %d, want 201", got)
	}

	entries, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var created int
	for _, e := range entries {
		if e.Action == auditActionCITrustCreated {
			created++
			if !strings.Contains(e.Detail, `"issuer"`) {
				t.Errorf("trust audit row has no issuer: %s", e.Detail)
			}
		}
	}
	if created != 3 {
		t.Fatalf("trust create audit rows = %d, want 3", created)
	}
}

func TestCITrustUniquePerProviderAndIssuer(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	post := func(body string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/ci/trusts", body))
		return rec.Code
	}
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"repository":"acme/web","ref_pattern":"refs/heads/main"}`, http.StatusCreated},
		{`{"provider":"gitlab","repository":"acme/web","ref_pattern":"refs/heads/main"}`, http.StatusCreated},
		{`{"provider":"gitlab","issuer":"https://gitlab.example.com","repository":"acme/web","ref_pattern":"refs/heads/main"}`, http.StatusCreated},
		{`{"provider":"gitlab","repository":"acme/web","ref_pattern":"refs/heads/main"}`, http.StatusConflict},
	} {
		if got := post(c.body); got != c.want {
			t.Errorf("%s = %d, want %d", c.body, got, c.want)
		}
	}
}
