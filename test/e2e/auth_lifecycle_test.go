package e2e

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
)

// authResponse mirrors internal/api's unexported loginResponse JSON shape
// (POST /auth/register and POST /auth/login both return it). internal/api
// deliberately doesn't export it: this test package sits on the far side
// of a real HTTP boundary, so it decodes the actual wire response into its
// own minimal copy of the contract rather than reaching across the
// package boundary, the same way any real API client would.
type authResponse struct {
	Username string `json:"username"`
}

// sessionResponse mirrors internal/api's unexported sessionInfoResponse
// (GET /auth/session), for the same reason as authResponse above.
type sessionResponse struct {
	Username  string `json:"username"`
	ExpiresAt string `json:"expires_at"`
}

// sessionCookieName mirrors internal/api/auth.go's own unexported
// sessionCookieName constant ("session_token"). Duplicated here rather
// than imported for the same package-boundary reason as authResponse.
const sessionCookieName = "session_token"

// TestAuthLifecycle_Live is the whole-chain proof internal/api/auth_test.go
// and internal/api/account_test.go's handler-level unit tests don't
// themselves give: that register, login, change-password, and
// revoke-others actually compose into one working admin auth flow when
// driven the way a real browser would, over real HTTP, against a real
// *api.Router wired to a real temp-file store.DB, with a real
// net/http.Client and cookie jar carrying session state exactly the way a
// browser's own cookie jar would. Every assertion below is against real
// HTTP status codes and real response bodies the server actually sent,
// never internal Go state (rt.sessions, the store) read directly, which
// is what the existing unit tests in internal/api already cover per
// handler.
//
// This test has nothing to do with containers, unlike this package's
// other live tests (TestDeploy_Live_BuildToHTTPS,
// TestWebhook_Live_PushToRunningContainer): it doesn't skip on missing
// Docker or BuildKit, because nothing here touches either.
func TestAuthLifecycle_Live(t *testing.T) {
	db := openLiveStore(t)
	b := &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := api.NewRouter(logger, b, db)

	// httptest.NewTLSServer, deliberately not the plain-HTTP NewServer:
	// handleLogin and handleRegister (internal/api/auth.go) set the
	// session cookie with Secure: true unconditionally, and Go's
	// net/http/cookiejar correctly refuses to attach a Secure cookie to a
	// request over a plain http:// connection, exactly like a real
	// browser would. A plain httptest.NewServer would silently defeat the
	// point of this test: the jar would store the Set-Cookie from
	// register/login but then never send it back, so every later
	// "authenticated" request would 401 for a reason that has nothing to
	// do with the auth logic actually under test.
	server := httptest.NewTLSServer(router.Handler())
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", server.URL, err)
	}

	const (
		username    = "admin"
		oldPassword = "first-real-password"
		newPassword = "second-real-password"
	)

	// Step 1: register the first admin. No admin row exists yet in this
	// fresh store, so this exercises the real interactive first-run path
	// (handleRegister), not BootstrapAdmin's env-var shortcut.
	clientA := newAuthClient(t, server)
	status, body := requestJSON(t, clientA, http.MethodPost, server.URL+"/api/v1/auth/register",
		`{"username":"`+username+`","password":"`+oldPassword+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d, body = %s", status, http.StatusCreated, body)
	}
	var registered authResponse
	if err := json.Unmarshal(body, &registered); err != nil {
		t.Fatalf("register: decode body %s: %v", body, err)
	}
	if registered.Username != username {
		t.Errorf("register: Username = %q, want %q", registered.Username, username)
	}
	if !hasSessionCookie(t, clientA, serverURL) {
		t.Fatal("register: no session cookie was set on the real response")
	}

	// Step 2: the session register just created actually authenticates a
	// real follow-up request, and the server's own record of who's signed
	// in (not this test's local assumption) matches.
	status, body = requestJSON(t, clientA, http.MethodGet, server.URL+"/api/v1/auth/session", "")
	if status != http.StatusOK {
		t.Fatalf("get session (A, post-register): status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var sessA sessionResponse
	if err := json.Unmarshal(body, &sessA); err != nil {
		t.Fatalf("get session (A, post-register): decode body %s: %v", body, err)
	}
	if sessA.Username != username {
		t.Errorf("get session (A, post-register): Username = %q, want %q", sessA.Username, username)
	}

	// Step 3: change the password from that same session.
	status, body = requestJSON(t, clientA, http.MethodPut, server.URL+"/api/v1/auth/password",
		`{"current_password":"`+oldPassword+`","new_password":"`+newPassword+`"}`)
	if status != http.StatusNoContent {
		t.Fatalf("change password: status = %d, want %d, body = %s", status, http.StatusNoContent, body)
	}

	// Step 4: a completely fresh cookie jar, carrying no residual cookie
	// from clientA, proves the OLD password genuinely stopped working
	// against the real server, and the NEW one genuinely works, rather
	// than this test's own local bookkeeping.
	clientB := newAuthClient(t, server)
	status, body = requestJSON(t, clientB, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"username":"`+username+`","password":"`+oldPassword+`"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("login with old password: status = %d, want %d, body = %s", status, http.StatusUnauthorized, body)
	}
	if hasSessionCookie(t, clientB, serverURL) {
		t.Error("login with old password: a session cookie was set despite the rejected login")
	}

	status, body = requestJSON(t, clientB, http.MethodPost, server.URL+"/api/v1/auth/login",
		`{"username":"`+username+`","password":"`+newPassword+`"}`)
	if status != http.StatusOK {
		t.Fatalf("login with new password: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	if !hasSessionCookie(t, clientB, serverURL) {
		t.Fatal("login with new password: no session cookie was set on the real response")
	}

	// Step 5: two concurrent, independently-authenticated real sessions
	// now exist: A (opened at registration, survived the password change
	// because handleChangePassword only revokes other sessions) and B
	// (just opened above with the new password). Verified against the
	// real server here, not assumed from the two successful login
	// responses alone.
	status, body = requestJSON(t, clientA, http.MethodGet, server.URL+"/api/v1/auth/session", "")
	if status != http.StatusOK {
		t.Fatalf("get session (A, pre-revoke): status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	status, body = requestJSON(t, clientB, http.MethodGet, server.URL+"/api/v1/auth/session", "")
	if status != http.StatusOK {
		t.Fatalf("get session (B, pre-revoke): status = %d, want %d, body = %s", status, http.StatusOK, body)
	}

	// Step 6: revoke-others, issued from A. A must survive, since a
	// caller's own session is never included in "others". B must come
	// back genuinely dead server-side: clientB's cookie jar is left
	// completely untouched between here and its next request, so a 401
	// below can only mean the server itself revoked it, not that this
	// test forgot the cookie locally.
	status, body = requestJSON(t, clientA, http.MethodPost, server.URL+"/api/v1/auth/sessions/revoke-others", "")
	if status != http.StatusNoContent {
		t.Fatalf("revoke-others: status = %d, want %d, body = %s", status, http.StatusNoContent, body)
	}

	status, body = requestJSON(t, clientA, http.MethodGet, server.URL+"/api/v1/auth/session", "")
	if status != http.StatusOK {
		t.Fatalf("get session (A, post-revoke): status = %d, want %d (caller's own session must survive revoke-others), body = %s", status, http.StatusOK, body)
	}
	var sessAAfter sessionResponse
	if err := json.Unmarshal(body, &sessAAfter); err != nil {
		t.Fatalf("get session (A, post-revoke): decode body %s: %v", body, err)
	}
	if sessAAfter.Username != username {
		t.Errorf("get session (A, post-revoke): Username = %q, want %q", sessAAfter.Username, username)
	}

	status, body = requestJSON(t, clientB, http.MethodGet, server.URL+"/api/v1/auth/session", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("get session (B, post-revoke): status = %d, want %d (must be genuinely revoked server-side), body = %s", status, http.StatusUnauthorized, body)
	}
}

// newAuthClient builds an independent *http.Client, trusting server's TLS
// certificate and carrying its own fresh, empty cookie jar: the real
// mechanism a browser uses to carry a session cookie across requests, not
// a manual "grab the Set-Cookie header and reattach it" shortcut.
//
// Deliberately not "client := server.Client(); client.Jar = jar":
// httptest.Server.Client() returns the same cached *http.Client pointer
// on every call for a given server, so mutating its Jar field for a
// second caller (clientB) would silently repoint the first caller's
// (clientA) Jar too, since both variables would name the same struct.
// This test needs two genuinely independent sessions, so it builds two
// separate *http.Client values here, sharing only the Transport (which
// carries the TLS trust and is safe to share across clients) while each
// gets its own Jar.
func newAuthClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	return &http.Client{
		Transport: server.Client().Transport,
		Jar:       jar,
	}
}

// hasSessionCookie reports whether client's cookie jar holds a non-empty
// session cookie for serverURL: the real signal that a register/login
// response actually authenticated this client, the same thing a browser
// would check, not just that the handler happened to return a 2xx status.
func hasSessionCookie(t *testing.T, client *http.Client, serverURL *url.URL) bool {
	t.Helper()
	jar, ok := client.Jar.(*cookiejar.Jar)
	if !ok || jar == nil {
		t.Fatal("hasSessionCookie: client has no cookie jar")
	}
	for _, c := range jar.Cookies(serverURL) {
		if c.Name == sessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}

// requestJSON issues a real HTTP request through client and returns the
// real status code and response body, the same pair every assertion in
// TestAuthLifecycle_Live checks. body == "" sends no request body (GET
// and the revoke-others POST have nothing to send).
func requestJSON(t *testing.T, client *http.Client, method, url, body string) (status int, respBody []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader) //nolint:noctx // test helper, no context to plumb through, matching this package's doGet
	if err != nil {
		t.Fatalf("NewRequest(%s %s) error = %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Errorf("closing response body: %v", closeErr)
		}
	}()

	respBody, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	return resp.StatusCode, respBody
}
