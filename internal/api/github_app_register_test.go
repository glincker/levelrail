package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithGitHubApp builds a Router with a fake
// GitHubAppSecrets wired in (WithGitHubAppSecrets, the same "construct
// via the real Option" convention newTestRouterWithBackupSecrets already
// uses) and a fake GitHubAppClient assigned directly afterward: there is
// no WithGitHubAppClient option (NewRouter defaults githubAppClient to a
// real *githubapp.Client unconditionally, since it needs no
// configuration to construct), so tests override the unexported field
// directly, the same way builds_test.go overrides rt.fetch.
func newTestRouterWithGitHubApp(t *testing.T, secrets GitHubAppSecrets, client GitHubAppClient) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithGitHubAppSecrets(secrets))
	rt.githubAppClient = client
	return rt, db
}

func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
}

// testPrimaryDomain is the fixed primary domain every test in this file
// configures before exercising a route that needs one
// (controlPlaneBaseURL). A single constant, not a parameter: every call
// site here happens to want the same value, and a real test needing a
// different one can pass it inline via db.UpdateIngressSettings
// directly rather than this helper growing an unused-in-practice knob.
const testPrimaryDomain = "deploy.example.com"

func setPrimaryDomain(t *testing.T, db *store.DB) {
	t.Helper()
	settings, err := db.GetIngressSettings(context.Background())
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	settings.PrimaryDomain = testPrimaryDomain
	if err := db.UpdateIngressSettings(context.Background(), settings); err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}
}

func TestHandleStartGitHubAppRegistration_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGitHubAppSecrets: no master key
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGitHubAppRegistration_NoPrimaryDomain(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	// No primary domain configured: PrimaryDomain defaults empty.

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGitHubAppRegistration_AlreadyConnected(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 1, ClientID: "Iv1.x", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStartGitHubAppRegistration_Success(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html (GitHub's manifest flow needs a real browser form POST)", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="https://github.com/settings/apps/new?state=`) {
		t.Errorf("body does not post to github.com/settings/apps/new with a state param: %s", body)
	}
	if !strings.Contains(body, `name="manifest"`) {
		t.Errorf("body has no hidden manifest field: %s", body)
	}
	if !strings.Contains(body, "deploy.example.com") {
		t.Errorf("body does not reference the configured primary domain: %s", body)
	}
	if !strings.Contains(body, "Test Platform") {
		t.Errorf("body does not reference the brand name (testBrand().Name): %s", body)
	}
}

func TestHandleStartGitHubAppRegistration_NameOverride(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start?name="+url.QueryEscape("My Custom App"), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "My Custom App") {
		t.Errorf("body does not reference the overridden name: %s", body)
	}
	if strings.Contains(body, "Test Platform (") {
		t.Errorf("body still contains the default computed name, override did not take effect: %s", body)
	}
}

func TestHandleGetGitHubAppManifestPreview_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithGitHubAppSecrets: no master key
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/preview", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetGitHubAppManifestPreview_NoPrimaryDomain(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/preview", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetGitHubAppManifestPreview_AlreadyConnected(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 1, ClientID: "Iv1.x", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/preview", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleGetGitHubAppManifestPreview_Success also proves the preview
// never mints a pending registration state: unlike register/start,
// calling it must not consume the one in-flight slot githubAppState
// holds, so a dialog that fetches a preview and is then cancelled
// leaves a subsequent real register/start call free to begin its own.
func TestHandleGetGitHubAppManifestPreview_Success(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/preview", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"app_name":"Test Platform (deploy.example.com)"`,
		`"homepage_url":"https://deploy.example.com"`,
		`"callback_url":"https://deploy.example.com/api/v1/github-app/callback"`,
		`"setup_url":"https://deploy.example.com/api/v1/github-app/installed"`,
		`"webhook_url":"https://deploy.example.com/api/v1/github-app/webhook"`,
		`"webhook_active":false`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %s, want it to contain %q", body, want)
		}
	}

	rt.githubAppState.mu.Lock()
	pending := rt.githubAppState.state
	rt.githubAppState.mu.Unlock()
	if pending != "" {
		t.Error("preview must not have minted a pending registration state")
	}
}

func TestHandleGitHubAppCallback_MissingCode(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/callback?state=x", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGitHubAppCallback_MissingState(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/callback?code=abc", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGitHubAppCallback_InvalidState(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/callback?code=abc&state=never-issued", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleGitHubAppCallback_Success drives the real state store: it
// starts a registration first (to get a real, valid state value from
// githubAppState.begin, the same one handleGitHubAppCallback checks
// against), then completes the callback with it.
func TestHandleGitHubAppCallback_Success(t *testing.T) {
	fakeClient := &fakeGitHubAppClient{
		exchangeCreds: githubapp.Credentials{
			AppID: 42, ClientID: "Iv1.abc", ClientSecret: "csecret",
			WebhookSecret: "whsecret", PrivateKeyPEM: "pemdata", Slug: "my-app",
		},
	}
	fakeSecrets := newFakeGitHubAppSecrets()
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	startRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(startRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start", ""))
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, body = %s", startRec.Code, startRec.Body.String())
	}
	state := extractState(t, startRec.Body.String())

	cbRec := httptest.NewRecorder()
	target := "/api/v1/github-app/callback?code=the-code&state=" + url.QueryEscape(state)
	rt.Handler().ServeHTTP(cbRec, authedRequest(t, cookie, http.MethodGet, target, ""))
	if cbRec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302, body = %s", cbRec.Code, cbRec.Body.String())
	}
	loc := cbRec.Header().Get("Location")
	if !strings.Contains(loc, "github.com/apps/my-app/installations/new") {
		t.Errorf("Location = %q, want a redirect into GitHub's install-new page for the app's slug", loc)
	}
	if fakeClient.gotCode != "the-code" {
		t.Errorf("ExchangeManifestCode was called with code = %q, want %q", fakeClient.gotCode, "the-code")
	}

	conn, err := db.GetGitHubAppConnection(context.Background())
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if conn.AppID != 42 || conn.ClientID != "Iv1.abc" {
		t.Errorf("stored connection = %+v, unexpected", conn)
	}

	secretsKey := store.GitHubAppSecretsKey()
	if got, _ := fakeSecrets.Resolve(context.Background(), secretsKey, "client_secret"); got != "csecret" {
		t.Errorf("stored client_secret = %q, want %q", got, "csecret")
	}
	if got, _ := fakeSecrets.Resolve(context.Background(), secretsKey, "webhook_secret"); got != "whsecret" {
		t.Errorf("stored webhook_secret = %q, want %q", got, "whsecret")
	}
	if got, _ := fakeSecrets.Resolve(context.Background(), secretsKey, "private_key"); got != "pemdata" {
		t.Errorf("stored private_key = %q, want %q", got, "pemdata")
	}

	// The credential values must never appear in the callback's own
	// response body (a redirect has an empty body, but this guards
	// against a future change accidentally writing one).
	for _, secret := range []string{"csecret", "whsecret", "pemdata"} {
		if strings.Contains(cbRec.Body.String(), secret) {
			t.Errorf("callback response body contains a secret value %q", secret)
		}
	}
}

func TestHandleGitHubAppCallback_ExchangeError(t *testing.T) {
	fakeClient := &fakeGitHubAppClient{exchangeErr: errFakeSecretNotSet} // any error works here
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), fakeClient)
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	startRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(startRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/register/start", ""))
	state := extractState(t, startRec.Body.String())

	rec := httptest.NewRecorder()
	target := "/api/v1/github-app/callback?code=bad&state=" + url.QueryEscape(state)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGitHubAppInstalled_MissingInstallationID(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/installed", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGitHubAppInstalled_MalformedInstallationID(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	tests := []string{"not-a-number", "-1", "0", "1.5"}
	for _, v := range tests {
		rec := httptest.NewRecorder()
		target := "/api/v1/github-app/installed?installation_id=" + url.QueryEscape(v)
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("installation_id=%q: status = %d, want 400", v, rec.Code)
		}
	}
}

func TestHandleGitHubAppInstalled_NoConnection(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), &fakeGitHubAppClient{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/installed?installation_id=7", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleGitHubAppInstalled_AppIDMismatch proves the defense this
// handler's own doc comment describes: an installation_id that GitHub
// reports as belonging to a different App ID than the one this control
// plane has on file is rejected, not silently trusted and recorded.
func TestHandleGitHubAppInstalled_AppIDMismatch(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	pemKey := testRSAPrivateKeyPEM(t)
	if err := fakeSecrets.SetValue(context.Background(), store.GitHubAppSecretsKey(), "private_key", pemKey); err != nil {
		t.Fatalf("seed private_key: %v", err)
	}
	fakeClient := &fakeGitHubAppClient{
		installation: githubapp.InstallationInfo{ID: 7, AppID: 999, AccountLogin: "octocat"},
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/installed?installation_id=7", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (app id mismatch must be rejected), body = %s", rec.Code, rec.Body.String())
	}

	conn, err := db.GetGitHubAppConnection(context.Background())
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if conn.InstallationID != nil {
		t.Errorf("InstallationID = %v after a rejected mismatch, want nil (must not be recorded)", *conn.InstallationID)
	}
}

func TestHandleGitHubAppInstalled_Success(t *testing.T) {
	fakeSecrets := newFakeGitHubAppSecrets()
	pemKey := testRSAPrivateKeyPEM(t)
	if err := fakeSecrets.SetValue(context.Background(), store.GitHubAppSecretsKey(), "private_key", pemKey); err != nil {
		t.Fatalf("seed private_key: %v", err)
	}
	fakeClient := &fakeGitHubAppClient{
		installation: githubapp.InstallationInfo{ID: 7, AppID: 42, AccountLogin: "octocat"},
	}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	if err := db.SaveGitHubAppConnection(context.Background(), store.GitHubAppConnection{
		AppID: 42, ClientID: "Iv1.abc", CreatedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app/installed?installation_id=7", ""))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302, body = %s", rec.Code, rec.Body.String())
	}
	if fakeClient.gotInstallID != 7 {
		t.Errorf("GetInstallation called with installationID = %d, want 7", fakeClient.gotInstallID)
	}
	if fakeClient.gotAppJWT == "" {
		t.Error("GetInstallation called with an empty appJWT")
	}

	conn, err := db.GetGitHubAppConnection(context.Background())
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if conn.InstallationID == nil || *conn.InstallationID != 7 {
		t.Errorf("InstallationID = %v, want 7", conn.InstallationID)
	}
	if conn.AccountLogin == nil || *conn.AccountLogin != "octocat" {
		t.Errorf("AccountLogin = %v, want octocat", conn.AccountLogin)
	}
}

// extractState pulls the state value out of the manifest form's action
// URL, the same string handleGitHubAppCallback's ?state= must match.
func extractState(t *testing.T, formHTML string) string {
	t.Helper()
	const marker = "state="
	idx := strings.Index(formHTML, marker)
	if idx == -1 {
		t.Fatalf("no state= found in form html: %s", formHTML)
	}
	rest := formHTML[idx+len(marker):]
	end := strings.IndexAny(rest, `"&`)
	if end == -1 {
		t.Fatalf("could not find end of state value in form html: %s", formHTML)
	}
	state, err := url.QueryUnescape(rest[:end])
	if err != nil {
		t.Fatalf("url.QueryUnescape(%q) error = %v", rest[:end], err)
	}
	return state
}
