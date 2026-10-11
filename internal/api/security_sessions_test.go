package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func adminSessionForTest(t *testing.T, h *codeHarness) *http.Cookie {
	t.Helper()
	admin, err := h.db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	return sessionCookieForTest(t, h.rt, admin.ID)
}

func listSessionsFor(t *testing.T, h *codeHarness, caller *http.Cookie, query string) (int, securitySessionsResponse) {
	t.Helper()
	rec := h.send(http.MethodGet, "/api/v1/security/sessions"+query, "", "", caller)
	var out securitySessionsResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out
}

func TestSecuritySessions_RevokeAuthorization(t *testing.T) {
	h := newCodeHarness(t, false)
	other := &store.User{ID: "user_other", Email: "other@example.com", DisplayName: "Other", Abilities: []string{AbilityRead}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(context.Background(), *other); err != nil {
		t.Fatal(err)
	}
	own := h.userSession()
	theirs := sessionCookieForTest(t, h.rt, other.ID)
	admin := adminSessionForTest(t, h)

	code, mine := listSessionsFor(t, h, own, "")
	if code != http.StatusOK || len(mine.Sessions) != 1 || !mine.Sessions[0].Current || !mine.Self {
		t.Fatalf("own list = %d %+v", code, mine)
	}
	_, theirList := listSessionsFor(t, h, theirs, "")
	theirID := theirList.Sessions[0].ID

	tests := []struct {
		name   string
		caller *http.Cookie
		path   string
		want   int
	}{
		{"a user cannot list another account", own, "/api/v1/security/sessions?user_id=" + other.ID, http.StatusForbidden},
		{"a user cannot revoke another account's session by naming it", own, "/api/v1/security/sessions/" + theirID + "?user_id=" + other.ID, http.StatusForbidden},
		{"a user cannot revoke another account's session as their own", own, "/api/v1/security/sessions/" + theirID, http.StatusNotFound},
		{"an admin can list any account", admin, "/api/v1/security/sessions?user_id=" + other.ID, http.StatusOK},
		{"an admin can revoke any account's session", admin, "/api/v1/security/sessions/" + theirID + "?user_id=" + other.ID, http.StatusNoContent},
		{"a revoked session is gone", admin, "/api/v1/security/sessions/" + theirID + "?user_id=" + other.ID, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := http.MethodDelete
			if tt.want == http.StatusOK || tt.name == "a user cannot list another account" {
				method = http.MethodGet
			}
			if rec := h.send(method, tt.path, "", "", tt.caller); rec.Code != tt.want {
				t.Fatalf("%s %s = %d %s, want %d", method, tt.path, rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
	if !containsAction(h.auditActions(), store.AuditActionSessionRevoke) {
		t.Fatal("admin revoke was not audited")
	}
	if _, ok := h.rt.libSessions.Lookup(context.Background(), theirs.Value); ok {
		t.Fatal("revoked session still validates")
	}
}

func TestSecuritySessions_RevokeOthersKeepsCurrent(t *testing.T) {
	h := newCodeHarness(t, false)
	keep := h.userSession()
	gone := h.userSession()
	rec := h.send(http.MethodPost, "/api/v1/security/sessions/revoke-others", "{}", "", keep)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke others = %d %s", rec.Code, rec.Body.String())
	}
	ctx := context.Background()
	if _, ok := h.rt.libSessions.Lookup(ctx, keep.Value); !ok {
		t.Fatal("current session was revoked")
	}
	if _, ok := h.rt.libSessions.Lookup(ctx, gone.Value); ok {
		t.Fatal("other session survived")
	}
}

func TestSecuritySessions_TokenNeedsSensitiveAbility(t *testing.T) {
	h := newCodeHarness(t, false)
	tests := []struct {
		name      string
		abilities []string
		want      int
	}{
		{"read token is refused", []string{AbilityRead}, http.StatusForbidden},
		{"write:sensitive token lists its owner", []string{AbilityRead, AbilityWriteSensitive}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptestRequestWithToken(http.MethodGet, "/api/v1/security/sessions", &store.APIToken{ID: "tok_x", OwnerUserID: h.user.ID, Abilities: tt.abilities})
			h.rt.handleListSecuritySessions(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestAccountSecurity_TokenCannotTurnProtectionOff(t *testing.T) {
	h := newCodeHarness(t, false)
	tok := &store.APIToken{ID: "tok_x", OwnerUserID: h.user.ID, Abilities: []string{AbilityWriteSensitive}}
	tests := []struct {
		name string
		body string
		want int
	}{
		{"token may turn it on", `{"require_new_device_approval":true}`, http.StatusOK},
		{"token may not turn it off", `{"require_new_device_approval":false}`, http.StatusForbidden},
		{"missing field", `{}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/security/account", stringsReader(tt.body))
			req = req.WithContext(withTokenIdentity(req.Context(), tok))
			rec := httptest.NewRecorder()
			h.rt.handlePutAccountSecurity(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
	s, err := h.db.GetUserSecuritySettings(context.Background(), h.user.ID)
	if err != nil || !s.RequireNewDeviceApproval {
		t.Fatalf("protection must stay on: %+v %v", s, err)
	}
}
