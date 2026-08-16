package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// sentEmailCall is one call captured by fakeEmailSender.
type sentEmailCall struct {
	to, subject, body string
}

// fakeEmailSender is a hand-written fake for email.Sender: handleForgotPassword
// sends in a detached background goroutine (see that handler's own doc
// comment for why), so tests observe the send through this channel
// rather than a synchronous return value.
type fakeEmailSender struct {
	calls chan sentEmailCall
	err   error
}

func newFakeEmailSender() *fakeEmailSender {
	return &fakeEmailSender{calls: make(chan sentEmailCall, 4)}
}

func (f *fakeEmailSender) Send(_ context.Context, to, subject, body string) error {
	f.calls <- sentEmailCall{to: to, subject: subject, body: body}
	return f.err
}

func newTestRouterWithEmailSender(t *testing.T, sender *fakeEmailSender) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithEmailSender(sender)), db
}

func waitForSend(t *testing.T, calls <-chan sentEmailCall) sentEmailCall {
	t.Helper()
	select {
	case c := <-calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handleForgotPassword's background send")
		return sentEmailCall{}
	}
}

func assertNoSend(t *testing.T, calls <-chan sentEmailCall) {
	t.Helper()
	select {
	case c := <-calls:
		t.Fatalf("unexpected email send: %+v", c)
	case <-time.After(150 * time.Millisecond):
	}
}

// tokenFromResetEmail extracts the raw token from the "?token=..." query
// param passwordResetURL embeds in the email body: the same value the
// frontend's reset-password page would read from its own query string.
func tokenFromResetEmail(t *testing.T, body string) string {
	t.Helper()
	idx := strings.Index(body, "token=")
	if idx == -1 {
		t.Fatalf("email body has no token= query param: %s", body)
	}
	rest := body[idx+len("token="):]
	end := strings.IndexAny(rest, "\n\r ")
	if end != -1 {
		rest = rest[:end]
	}
	token, err := url.QueryUnescape(rest)
	if err != nil {
		t.Fatalf("unescape token: %v", err)
	}
	return token
}

func forgotPasswordBody(email string) string {
	return `{"email":"` + email + `"}`
}

func TestHandleForgotPassword_AlwaysReturns204(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, db *store.DB)
	}{
		{name: "unknown email", setup: func(_ *testing.T, _ *store.DB) {}},
		{name: "known email, no sender configured", setup: func(t *testing.T, db *store.DB) { bootstrapTestAdmin(t, db) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t) // no WithEmailSender: nil is valid
			tt.setup(t, db)

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(forgotPasswordBody(testAdminUsername))))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			if rec.Body.Len() != 0 {
				t.Errorf("body = %q, want empty", rec.Body.String())
			}
		})
	}
}

func TestHandleForgotPassword_UnknownEmail_NeverSends(t *testing.T) {
	sender := newFakeEmailSender()
	rt, _ := newTestRouterWithEmailSender(t, sender)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(forgotPasswordBody("nobody@example.com"))))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	assertNoSend(t, sender.calls)
}

func TestHandleForgotPassword_SendsWhenConfigured(t *testing.T) {
	sender := newFakeEmailSender()
	rt, db := newTestRouterWithEmailSender(t, sender)
	bootstrapTestAdmin(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(forgotPasswordBody(testAdminUsername))))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	call := waitForSend(t, sender.calls)
	if call.to != testAdminUsername {
		t.Errorf("sent to %q, want %q", call.to, testAdminUsername)
	}
	if !strings.Contains(call.body, "token=") {
		t.Errorf("body has no reset link: %s", call.body)
	}
}

func TestHandleForgotPassword_MissingEmail(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(`{"email":""}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleForgotPassword_RateLimited(t *testing.T) {
	rt, _ := newTestRouter(t)

	var lastCode int
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(forgotPasswordBody(testAdminUsername))))
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("status of 5th rapid request = %d, want %d (rate limited)", lastCode, http.StatusTooManyRequests)
	}
}

func TestHandleResetPassword_FullRoundTrip(t *testing.T) {
	sender := newFakeEmailSender()
	rt, db := newTestRouterWithEmailSender(t, sender)
	bootstrapTestAdmin(t, db)

	// A pre-existing session, to prove reset revokes it.
	oldCookie := loginTestSession(t, rt, db)

	rt.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(forgotPasswordBody(testAdminUsername))))
	call := waitForSend(t, sender.calls)
	token := tokenFromResetEmail(t, call.body)

	const newPassword = "a-new-strong-password" //nolint:gosec // fake fixture, not a real credential
	resetBody := `{"token":"` + token + `","new_password":"` + newPassword + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(resetBody)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reset-password: status = %d, want %d, body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// New password works.
	loginOK := httptest.NewRecorder()
	loginOKBody := `{"username":"` + testAdminUsername + `","password":"` + newPassword + `"}`
	rt.Handler().ServeHTTP(loginOK, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginOKBody)))
	if loginOK.Code != http.StatusOK {
		t.Errorf("login with new password: status = %d, want %d, body = %s", loginOK.Code, http.StatusOK, loginOK.Body.String())
	}

	// Old password no longer works.
	loginOld := httptest.NewRecorder()
	loginOldBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rt.Handler().ServeHTTP(loginOld, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginOldBody)))
	if loginOld.Code != http.StatusUnauthorized {
		t.Errorf("login with old password: status = %d, want %d", loginOld.Code, http.StatusUnauthorized)
	}

	// The session that existed before the reset is revoked.
	oldSessRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(oldSessRec, authedRequest(t, oldCookie, http.MethodGet, "/api/v1/apps", ""))
	if oldSessRec.Code != http.StatusUnauthorized {
		t.Errorf("pre-reset session: status = %d, want %d (must be revoked)", oldSessRec.Code, http.StatusUnauthorized)
	}

	// The token is single-use: replaying it must fail.
	replay := httptest.NewRecorder()
	rt.Handler().ServeHTTP(replay, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(resetBody)))
	if replay.Code != http.StatusBadRequest {
		t.Errorf("replayed token: status = %d, want %d", replay.Code, http.StatusBadRequest)
	}
}

func TestHandleResetPassword_MissingToken(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(`{"token":"","new_password":"a-new-strong-password"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleResetPassword_PasswordTooShort(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(`{"token":"x","new_password":"short"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleResetPassword_MalformedBody(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(`{not json`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleResetPassword_UnknownToken_GenericError(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)

	rec := httptest.NewRecorder()
	body := `{"token":"totally-made-up","new_password":"a-new-strong-password"}`
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errInvalidOrExpiredResetToken.Error()) {
		t.Errorf("body = %s, want the generic invalid/expired message", rec.Body.String())
	}
}

func TestHandleResetPassword_ExpiredToken_SameGenericError(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	user, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}

	const plaintext = "expired-token-value" //nolint:gosec // fake fixture, not a real credential
	now := time.Now().UTC()
	if err := db.SavePasswordResetToken(context.Background(), store.PasswordResetToken{
		ID: "prt_expired", UserID: user.ID, TokenHash: hashToken(plaintext), CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed expired token: %v", err)
	}

	rec := httptest.NewRecorder()
	body := `{"token":"` + plaintext + `","new_password":"a-new-strong-password"}`
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), errInvalidOrExpiredResetToken.Error()) {
		t.Errorf("body = %s, want the same generic message an unknown token gets", rec.Body.String())
	}
}

func TestHandleResetPassword_NotGatedBehindAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	bootstrapTestAdmin(t, db)
	// No cookie, no bearer token at all: this must reach the handler
	// (and fail on token validity, not on authentication) since the
	// whole point of the endpoint is to work while unauthenticated.
	rec := httptest.NewRecorder()
	body := `{"token":"whatever","new_password":"a-new-strong-password"}`
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body)))
	if rec.Code == http.StatusUnauthorized {
		t.Error("status = 401: reset-password must not require authentication")
	}
}
