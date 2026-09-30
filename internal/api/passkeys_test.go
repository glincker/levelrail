package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/go-webauthn/webauthn/webauthn"
)

// testPasskeyOrigin/testPasskeyRPID mirror httptest.NewRequest's own
// default host ("example.com") for a target with no explicit host: that
// default is what passkeyRelyingParty derives its RPID/origin from, so
// the virtual authenticator below has to agree with it exactly for a
// ceremony to verify.
const (
	testPasskeyRPID   = "example.com"
	testPasskeyOrigin = "http://example.com" //nolint:gosec // test-fixture origin string, not a credential
)

func passkeyRelyingPartyForTest(rt *Router) virtualwebauthn.RelyingParty {
	return virtualwebauthn.RelyingParty{Name: rt.brand.Name, ID: testPasskeyRPID, Origin: testPasskeyOrigin}
}

// beginPasskeyRegistration drives the real begin handler and parses its
// response into the shape virtualwebauthn needs to build a response.
func beginPasskeyRegistration(t *testing.T, rt *Router, cookie *http.Cookie) (sessionID string, options *virtualwebauthn.AttestationOptions) {
	t.Helper()
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/passkeys/register/begin", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("register begin: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp passkeyRegistrationBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal register begin response: %v", err)
	}
	optJSON, err := json.Marshal(resp.Options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(string(optJSON))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	return resp.SessionID, parsed
}

func finishPasskeyRegistrationRequest(t *testing.T, rt *Router, cookie *http.Cookie, sessionID, label, body string) *http.Response {
	t.Helper()
	target := "/api/v1/auth/passkeys/register/finish?session_id=" + url.QueryEscape(sessionID)
	if label != "" {
		target += "&label=" + url.QueryEscape(label)
	}
	rec := doJSON(t, rt, http.MethodPost, target, body, cookie)
	return rec.Result()
}

// registerTestPasskey drives a complete, real registration ceremony
// (begin -> a virtual authenticator's response -> finish) against the
// live HTTP handlers, the same round trip a browser and a real
// authenticator would perform.
func registerTestPasskey(t *testing.T, rt *Router, cookie *http.Cookie, label string) (virtualwebauthn.Credential, virtualwebauthn.Authenticator) {
	t.Helper()
	sessionID, options := beginPasskeyRegistration(t, rt, cookie)
	rp := passkeyRelyingPartyForTest(rt)
	authenticator := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	body := virtualwebauthn.CreateAttestationResponse(rp, authenticator, cred, *options)

	resp := finishPasskeyRegistrationRequest(t, rt, cookie, sessionID, label, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register finish: status = %d", resp.StatusCode)
	}
	authenticator.AddCredential(cred)
	return cred, authenticator
}

func TestPasskeyRegistration_FullFlow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	registerTestPasskey(t, rt, cookie, "My Test Key")

	rec := doJSON(t, rt, http.MethodGet, "/api/v1/auth/passkeys", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list []passkeyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0].Label != "My Test Key" {
		t.Errorf("Label = %q, want %q", list[0].Label, "My Test Key")
	}
	if list[0].LastUsedAt != nil {
		t.Errorf("LastUsedAt = %v, want nil before any login", list[0].LastUsedAt)
	}
}

func TestPasskeyRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/auth/passkeys"},
		{http.MethodPost, "/api/v1/auth/passkeys/register/begin"},
		{http.MethodPost, "/api/v1/auth/passkeys/register/finish"},
		{http.MethodDelete, "/api/v1/auth/passkeys/pk_1"},
	})
}

// TestPasskeyRegistration_ReplayedSessionRejected is the required
// "a stale/reused challenge is rejected" test for the registration
// ceremony: the exact same attestation response, submitted a second
// time against the same session_id, must fail, because
// passkeyCeremonyStore.consume already popped that entry on the first
// (successful) finish call.
func TestPasskeyRegistration_ReplayedSessionRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	sessionID, options := beginPasskeyRegistration(t, rt, cookie)
	rp := passkeyRelyingPartyForTest(rt)
	authenticator := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	body := virtualwebauthn.CreateAttestationResponse(rp, authenticator, cred, *options)

	first := finishPasskeyRegistrationRequest(t, rt, cookie, sessionID, "Key", body)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first finish: status = %d", first.StatusCode)
	}

	second := finishPasskeyRegistrationRequest(t, rt, cookie, sessionID, "Key", body)
	if second.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed finish: status = %d, want %d", second.StatusCode, http.StatusUnauthorized)
	}
}

func TestPasskeyRegistration_UnknownSessionRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	resp := finishPasskeyRegistrationRequest(t, rt, cookie, "not-a-real-session-id", "Key", "{}")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestPasskeyDelete_RemovesCredential(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	registerTestPasskey(t, rt, cookie, "Key")

	rec := doJSON(t, rt, http.MethodGet, "/api/v1/auth/passkeys", "", cookie)
	var list []passkeyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	rec = doJSON(t, rt, http.MethodDelete, "/api/v1/auth/passkeys/"+list[0].ID, "", cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/auth/passkeys", "", cookie)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("len(list) after delete = %d, want 0", len(list))
	}
}

func TestPasskeyDelete_UnknownIDNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := doJSON(t, rt, http.MethodDelete, "/api/v1/auth/passkeys/pk_does_not_exist", "", cookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestPasskeyDelete_ScopedToOwner(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	registerTestPasskey(t, rt, cookie, "Key")

	rec := doJSON(t, rt, http.MethodGet, "/api/v1/auth/passkeys", "", cookie)
	var list []passkeyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}

	other := storeUserForTest(t, db, "other@example.com")
	otherCookie := sessionCookieForTest(t, rt, other.ID)

	rec = doJSON(t, rt, http.MethodDelete, "/api/v1/auth/passkeys/"+list[0].ID, "", otherCookie)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete as different user: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/auth/passkeys", "", cookie)
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list after cross-account delete attempt: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("credential was removed by a different account: len(list) = %d, want 1", len(list))
	}
}

// loginWithPasskey drives the full public login ceremony
// (begin -> virtual authenticator response -> finish) for username,
// using a credential already registered via registerTestPasskey.
func loginWithPasskey(t *testing.T, rt *Router, username string, cred virtualwebauthn.Credential, authenticator virtualwebauthn.Authenticator) *http.Response {
	t.Helper()
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"`+username+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login begin: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp passkeyLoginBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login begin response: %v", err)
	}
	optJSON, err := json.Marshal(resp.Options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	assertionOptions, err := virtualwebauthn.ParseAssertionOptions(string(optJSON))
	if err != nil {
		t.Fatalf("parse assertion options: %v", err)
	}

	rp := passkeyRelyingPartyForTest(rt)
	body := virtualwebauthn.CreateAssertionResponse(rp, authenticator, cred, *assertionOptions)

	target := "/api/v1/auth/passkey-login/finish?session_id=" + url.QueryEscape(resp.SessionID)
	finishRec := doJSON(t, rt, http.MethodPost, target, body, nil)
	return finishRec.Result()
}

func TestPasskeyLogin_FullFlow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cred, authenticator := registerTestPasskey(t, rt, cookie, "Key")

	resp := loginWithPasskey(t, rt, testAdminUsername, cred, authenticator)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login finish: status = %d", resp.StatusCode)
	}
	var loginResp loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Email != testAdminUsername {
		t.Errorf("Email = %q, want %q", loginResp.Email, testAdminUsername)
	}
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("no session cookie set after a successful passkey login")
	}
}

func TestPasskeyLogin_UnknownUsername_GenericError(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"nobody@example.com"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestPasskeyLogin_NoPasskeyRegistered_SameErrorAsUnknownUsername proves
// the two failure cases are indistinguishable to the caller, the same
// property handleLogin's own "invalid credentials" message gives an
// unknown email vs. a wrong password.
func TestPasskeyLogin_NoPasskeyRegistered_SameErrorAsUnknownUsername(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"`+testAdminUsername+`"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	unknownRec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"nobody@example.com"}`, nil)
	if rec.Body.String() != unknownRec.Body.String() {
		t.Errorf("body = %q, want the same generic message as an unknown username %q", rec.Body.String(), unknownRec.Body.String())
	}
}

// TestPasskeyLogin_ReplayedAssertionRejected is the login ceremony's own
// stale/reused-challenge test, the counterpart to
// TestPasskeyRegistration_ReplayedSessionRejected.
func TestPasskeyLogin_ReplayedAssertionRejected(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	cred, authenticator := registerTestPasskey(t, rt, cookie, "Key")

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"`+testAdminUsername+`"}`, nil)
	var resp passkeyLoginBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login begin response: %v", err)
	}
	optJSON, err := json.Marshal(resp.Options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	assertionOptions, err := virtualwebauthn.ParseAssertionOptions(string(optJSON))
	if err != nil {
		t.Fatalf("parse assertion options: %v", err)
	}
	rp := passkeyRelyingPartyForTest(rt)
	body := virtualwebauthn.CreateAssertionResponse(rp, authenticator, cred, *assertionOptions)
	target := "/api/v1/auth/passkey-login/finish?session_id=" + url.QueryEscape(resp.SessionID)

	first := doJSON(t, rt, http.MethodPost, target, body, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first login finish: status = %d, body = %s", first.Code, first.Body.String())
	}
	second := doJSON(t, rt, http.MethodPost, target, body, nil)
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replayed login finish: status = %d, want %d", second.Code, http.StatusUnauthorized)
	}
}

func TestPasskeyCeremonyStore_ConsumeIsSingleUse(t *testing.T) {
	s := newPasskeyCeremonyStore()
	session := webauthn.SessionData{Challenge: "test-challenge", UserID: []byte("user_1"), Expires: time.Now().Add(time.Minute)}
	token, err := s.create(session)
	if err != nil {
		t.Fatalf("create() error = %v", err)
	}
	if _, ok := s.consume(token); !ok {
		t.Fatal("first consume() ok = false, want true")
	}
	if _, ok := s.consume(token); ok {
		t.Fatal("second consume() of the same token ok = true, want false (replay must be rejected)")
	}
}

func TestPasskeyCeremonyStore_UnknownTokenRejected(t *testing.T) {
	s := newPasskeyCeremonyStore()
	if _, ok := s.consume("never-issued"); ok {
		t.Fatal("consume() of an unissued token ok = true, want false")
	}
}

func TestPasskeyCeremonyStore_ExpiredEntryRejected(t *testing.T) {
	s := newPasskeyCeremonyStore()
	token, err := s.create(webauthn.SessionData{Challenge: "test-challenge"})
	if err != nil {
		t.Fatalf("create() error = %v", err)
	}
	// Simulate the TTL already having elapsed rather than sleeping in
	// the test.
	s.mu.Lock()
	e := s.entries[token]
	e.expiresAt = time.Now().Add(-time.Second)
	s.entries[token] = e
	s.mu.Unlock()

	if _, ok := s.consume(token); ok {
		t.Fatal("consume() of an expired token ok = true, want false")
	}
}
