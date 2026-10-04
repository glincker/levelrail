package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// mintSessionLink mints a session link through the real handler, either
// with a session cookie (cookie non-nil) or a bearer token (bearer
// non-empty), and returns the decoded response.
func mintSessionLink(t *testing.T, rt *Router, cookie *http.Cookie, bearerTok string) (mintSessionLinkResponse, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session-links", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if bearerTok != "" {
		req.Header.Set("Authorization", "Bearer "+bearerTok)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		return mintSessionLinkResponse{}, rec
	}
	var out mintSessionLinkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode mint response: %v, body = %s", err, rec.Body.String())
	}
	return out, rec
}

func consumeSessionLinkRequest(t *testing.T, rt *Router, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session-links/"+token+"/consume", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func sessionCookieFromRecorder(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatalf("no session cookie in response: %v", rec.Result().Cookies())
	return nil
}

func seedRootAPIToken(t *testing.T, db *store.DB, id string, abilities []string) string {
	t.Helper()
	plain := "session-link-secret-" + id
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: id, Name: "agent-token-" + id, TokenHash: hashToken(plain), Abilities: abilities,
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	return plain
}

func TestHandleMintSessionLink_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	readOnly := storeUserWithAbilitiesForTest(t, db, "read-only@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, readOnly.ID)

	_, rec := mintSessionLink(t, rt, cookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (read-only session must not mint)", rec.Code, http.StatusForbidden)
	}
}

func TestHandleMintSessionLink_NotGatedBehindAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	_, rec := mintSessionLink(t, rt, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (no caller at all)", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleMintSessionLink_RootSession_ReturnsTokenAndURL(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	out, rec := mintSessionLink(t, rt, cookie, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if out.Token == "" {
		t.Error("Token is empty")
	}
	if !strings.Contains(out.URL, "session_link="+out.Token) {
		t.Errorf("URL = %q, want it to embed the token", out.URL)
	}
}

func TestHandleMintSessionLink_RootToken_Succeeds(t *testing.T) {
	rt, db := newTestRouter(t)
	plain := seedRootAPIToken(t, db, "tok_root", []string{AbilityRoot})

	out, rec := mintSessionLink(t, rt, nil, plain)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if out.Token == "" {
		t.Error("Token is empty")
	}
}

// TestSessionLink_UserPrincipal_FullRoundTrip proves a user-minted link
// establishes a normal session for that same user: it can reach an
// AbilityRead route, and GET /api/v1/auth/session (requireAuth,
// session-only) succeeds, exactly as an ordinary login would.
func TestSessionLink_UserPrincipal_FullRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	minted, mintRec := mintSessionLink(t, rt, cookie, "")
	if mintRec.Code != http.StatusCreated {
		t.Fatalf("mint: status = %d, body = %s", mintRec.Code, mintRec.Body.String())
	}

	consumeRec := consumeSessionLinkRequest(t, rt, minted.Token)
	if consumeRec.Code != http.StatusOK {
		t.Fatalf("consume: status = %d, body = %s", consumeRec.Code, consumeRec.Body.String())
	}
	newCookie := sessionCookieFromRecorder(t, consumeRec)

	appsRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(appsRec, authedRequest(t, newCookie, http.MethodGet, "/api/v1/apps", ""))
	if appsRec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/apps with consumed session: status = %d, want %d", appsRec.Code, http.StatusOK)
	}

	sessRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(sessRec, authedRequest(t, newCookie, http.MethodGet, "/api/v1/auth/session", ""))
	if sessRec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/auth/session with consumed session: status = %d, want %d, body = %s", sessRec.Code, http.StatusOK, sessRec.Body.String())
	}
}

// TestSessionLink_TokenPrincipal_InheritsMintingAbilities proves the
// consumed session's access is capped at exactly the minting token's own
// abilities (AbilityRead only here): it can reach an AbilityRead route
// but gets 403, not 200, on an AbilityWrite one.
func TestSessionLink_TokenPrincipal_InheritsMintingAbilities(t *testing.T) {
	rt, db := newTestRouter(t)
	plain := seedRootAPIToken(t, db, "tok_root", []string{AbilityRoot})
	minted, mintRec := mintSessionLink(t, rt, nil, plain)
	if mintRec.Code != http.StatusCreated {
		t.Fatalf("mint: status = %d, body = %s", mintRec.Code, mintRec.Body.String())
	}

	consumeRec := consumeSessionLinkRequest(t, rt, minted.Token)
	if consumeRec.Code != http.StatusOK {
		t.Fatalf("consume: status = %d, body = %s", consumeRec.Code, consumeRec.Body.String())
	}
	newCookie := sessionCookieFromRecorder(t, consumeRec)

	// Root-minted: AbilityWriteSensitive route (restricted to root in
	// this codebase) must still succeed, proving abilities carried over,
	// not just "any session works."
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, newCookie, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/apps with root-minted pinned session: status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestSessionLink_TokenPrincipal_CarriesOriginalTokenIdentity proves the
// consumed pinned session isn't just "a generic root session": a
// resource-scoped IAM Deny attached to the *original minting token's own
// ID* still applies to it, exactly as it would to the token itself
// (requireAbilityForResource's PrincipalID comes from the pinned
// session, not a fresh identity). An unrelated app, with no Deny, still
// works, isolating the effect to the one resource the policy names.
func TestSessionLink_TokenPrincipal_CarriesOriginalTokenIdentity(t *testing.T) {
	rt, db := newTestRouter(t)
	seedWebAppForTest(t, db)
	plain := seedRootAPIToken(t, db, "tok_root", []string{AbilityRoot})
	attachTestPolicy(t, db, "deny-web", "Deny", "*", "app:web", store.PrincipalTypeToken, "tok_root")

	minted, mintRec := mintSessionLink(t, rt, nil, plain)
	if mintRec.Code != http.StatusCreated {
		t.Fatalf("mint: status = %d, body = %s", mintRec.Code, mintRec.Body.String())
	}
	consumeRec := consumeSessionLinkRequest(t, rt, minted.Token)
	if consumeRec.Code != http.StatusOK {
		t.Fatalf("consume: status = %d, body = %s", consumeRec.Code, consumeRec.Body.String())
	}
	newCookie := sessionCookieFromRecorder(t, consumeRec)

	deniedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(deniedRec, authedRequest(t, newCookie, http.MethodGet, "/api/v1/apps/web", ""))
	if deniedRec.Code != http.StatusForbidden {
		t.Errorf("GET /api/v1/apps/web (Deny'd to the minting token): status = %d, want %d", deniedRec.Code, http.StatusForbidden)
	}

	allowedRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(allowedRec, authedRequest(t, newCookie, http.MethodGet, "/api/v1/apps", ""))
	if allowedRec.Code != http.StatusOK {
		t.Errorf("GET /api/v1/apps (unscoped list, no Deny): status = %d, want %d", allowedRec.Code, http.StatusOK)
	}
}

func TestConsumeSessionLink_SecondAttemptFails(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	minted, _ := mintSessionLink(t, rt, cookie, "")

	first := consumeSessionLinkRequest(t, rt, minted.Token)
	if first.Code != http.StatusOK {
		t.Fatalf("first consume: status = %d, body = %s", first.Code, first.Body.String())
	}

	second := consumeSessionLinkRequest(t, rt, minted.Token)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second consume (replay): status = %d, want %d", second.Code, http.StatusBadRequest)
	}
}

func TestConsumeSessionLink_UnknownTokenRejected(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := consumeSessionLinkRequest(t, rt, "totally-made-up")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestConsumeSessionLink_ExpiredRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	user, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}

	const plaintext = "expired-session-link-value" //nolint:gosec // fake fixture, not a real credential
	now := time.Now().UTC()
	if err := db.SaveSessionLinkToken(context.Background(), store.SessionLinkToken{
		ID: "sl_expired", PrincipalType: store.PrincipalTypeUser, PrincipalID: user.ID,
		Abilities: user.Abilities, DisplayName: user.DisplayName, TokenHash: hashToken(plaintext),
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed expired token: %v", err)
	}

	rec := consumeSessionLinkRequest(t, rt, plaintext)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestConsumeSessionLink_NotGatedBehindAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := consumeSessionLinkRequest(t, rt, "whatever")
	if rec.Code == http.StatusUnauthorized {
		t.Error("status = 401: consuming a session link must not itself require authentication")
	}
}

func TestHandleConsumeSessionLink_MissingToken404sOrBadRequest(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/session-links//consume", nil))
	if rec.Code == http.StatusOK {
		t.Errorf("status = %d, want a failure for an empty token segment", rec.Code)
	}
}
