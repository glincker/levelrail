package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTokenCleanupEndpoint(t *testing.T) {
	rt, db := newTestRouter(t)
	adminCookie := loginTestSession(t, rt, db)
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRead})
	aliceCookie := sessionCookieForTest(t, rt, alice.ID)

	active := mintTokenAs(t, rt, adminCookie, "active")
	oldRevoked := mintTokenAs(t, rt, adminCookie, "old-revoked")
	freshRevoked := mintTokenAs(t, rt, adminCookie, "fresh-revoked")
	for _, id := range []string{oldRevoked.ID, freshRevoked.ID} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, adminCookie, http.MethodDelete, "/api/v1/auth/tokens/"+id, ""))
		if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
			t.Fatalf("revoke %s: status = %d", id, rec.Code)
		}
	}
	old := time.Now().Add(-100 * 24 * time.Hour)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ?`, old.UTC().Format(time.RFC3339Nano), oldRevoked.ID); err != nil {
		t.Fatalf("backdate mirror: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE theauth_api_tokens SET revoked_at = ? WHERE id = (SELECT engine_id FROM authengine_token_map WHERE legacy_id = ?)`, old.UnixMicro(), oldRevoked.ID); err != nil {
		t.Fatalf("backdate library: %v", err)
	}

	post := func(cookie *http.Cookie, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens/cleanup", body))
		return rec
	}

	if rec := post(aliceCookie, `{"older_than_days":30}`); rec.Code != http.StatusForbidden {
		t.Errorf("non-root status = %d, want 403", rec.Code)
	}
	for _, body := range []string{`{}`, `{"older_than_days":-1}`, `{"older_than_days":99999}`, `{"older_than_days":0}`, `{"older_than_days":0,"confirm":false}`} {
		if rec := post(adminCookie, body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s status = %d, want 400", body, rec.Code)
		}
	}

	if rec := post(adminCookie, `{}`); !strings.Contains(rec.Body.String(), `"field":"older_than_days"`) {
		t.Errorf("missing field body = %s, want a field error", rec.Body.String())
	}
	if rec := post(adminCookie, `{"older_than_days":0}`); !strings.Contains(rec.Body.String(), `"field":"confirm"`) {
		t.Errorf("unconfirmed zero body = %s, want a confirm field error", rec.Body.String())
	}

	rec := post(adminCookie, `{"older_than_days":30}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got tokenCleanupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Deleted != 1 {
		t.Fatalf("response = %s, want deleted 1 (%v)", rec.Body.String(), err)
	}
	ids := listTokenIDs(t, rt, adminCookie)
	if slices.Contains(ids, oldRevoked.ID) || !slices.Contains(ids, active.ID) || !slices.Contains(ids, freshRevoked.ID) {
		t.Errorf("ids after cleanup = %v", ids)
	}

	rec = post(adminCookie, `{"older_than_days":0,"confirm":true}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Deleted != 1 {
		t.Fatalf("explicit 0 response = %s, want the remaining revoked token", rec.Body.String())
	}
	if ids := listTokenIDs(t, rt, adminCookie); !slices.Contains(ids, active.ID) {
		t.Errorf("active token was removed: %v", ids)
	}
}
