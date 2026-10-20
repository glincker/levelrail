package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/totp"
	"github.com/descope/virtualwebauthn"
)

const mfaTestOrigin = "http://example.com" //nolint:gosec // test-fixture origin, not a credential

type mfaHarness struct {
	t      *testing.T
	rt     *Router
	db     *store.DB
	key    []byte
	cookie *http.Cookie
	userID string
	secret string
}

// newMFAHarness builds a router over a fresh database holding the bootstrap admin.
func newMFAHarness(t *testing.T) *mfaHarness {
	t.Helper()
	db := openTestDB(t)
	h := &mfaHarness{t: t, db: db, key: make([]byte, 32)}
	if _, err := rand.Read(h.key); err != nil {
		t.Fatal(err)
	}
	bootstrapTestAdmin(t, db)
	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	h.userID = admin.ID
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL: mfaTestOrigin, EncryptionKey: h.key, TOTPIssuer: "test", TokenPrefix: "tk",
		Directory: authengine.NewDirectory(db.DB), MFA: authengine.MFAConfig{DashboardURL: mfaTestOrigin},
		Sessions: authengine.SessionsHooks{Mail: &authengine.MailRelay{}},
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	h.rt = NewRouter(discardLogger(), testBrand(), db, WithAuthEngine(eng))
	h.cookie = loginTestSession(t, h.rt, db)
	return h
}

func doJSON(t *testing.T, rt *Router, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *mfaHarness) do(method, path, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	return doJSON(h.t, h.rt, method, path, body, h.cookie)
}

func (h *mfaHarness) code(offset time.Duration) string {
	h.t.Helper()
	c, err := totp.GenerateCode(h.secret, time.Now().Add(offset))
	if err != nil {
		h.t.Fatalf("GenerateCode: %v", err)
	}
	return c
}

func (h *mfaHarness) passwordLogin() *httptest.ResponseRecorder {
	h.t.Helper()
	return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/login",
		`{"username":"`+testAdminUsername+`","password":"`+testAdminPassword+`"}`, nil)
}

func (h *mfaHarness) verifyLogin(mfaToken, code string) *httptest.ResponseRecorder {
	h.t.Helper()
	body := fmt.Sprintf(`{"mfa_token":%q,"code":%q}`, mfaToken, code)
	return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/2fa/verify", body, nil)
}

func (h *mfaHarness) mfaToken(rec *httptest.ResponseRecorder) string {
	h.t.Helper()
	var resp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || !resp.MFARequired || resp.MFAToken == "" {
		h.t.Fatalf("login must ask for a second factor: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return resp.MFAToken
}

// enrollTOTP drives setup and confirm for the session user.
func (h *mfaHarness) enrollTOTP() {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/auth/2fa/setup", "")
	var setup twoFactorSetupResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &setup) != nil {
		h.t.Fatalf("setup: %d %s", rec.Code, rec.Body.String())
	}
	h.secret = setup.Secret
	rec = h.do(http.MethodPost, "/api/v1/auth/2fa/confirm", `{"code":"`+h.code(0)+`"}`)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("confirm: %d %s", rec.Code, rec.Body.String())
	}
}

type softKey struct {
	cred virtualwebauthn.Credential
	auth virtualwebauthn.Authenticator
}

func (h *mfaHarness) rp() virtualwebauthn.RelyingParty {
	return virtualwebauthn.RelyingParty{Name: h.rt.brand.Name, ID: "example.com", Origin: mfaTestOrigin}
}

func (h *mfaHarness) registerBegin() (string, *virtualwebauthn.AttestationOptions) {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/auth/passkeys/register/begin", "")
	if rec.Code != http.StatusOK {
		h.t.Fatalf("register begin: %d %s", rec.Code, rec.Body.String())
	}
	var resp passkeyRegistrationBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		h.t.Fatal(err)
	}
	raw, _ := json.Marshal(resp.Options)
	opts, err := virtualwebauthn.ParseAttestationOptions(string(raw))
	if err != nil {
		h.t.Fatalf("parse attestation options: %v", err)
	}
	return resp.SessionID, opts
}

func (h *mfaHarness) registerFinish(sessionID, label, body string) *httptest.ResponseRecorder {
	return h.do(http.MethodPost, "/api/v1/auth/passkeys/register/finish?session_id="+url.QueryEscape(sessionID)+"&label="+url.QueryEscape(label), body)
}

// registerPasskey runs a full registration with a software authenticator that, like a real
// one, remembers the user handle the server asked it to store.
func (h *mfaHarness) registerPasskey(label string) softKey {
	h.t.Helper()
	sessionID, opts := h.registerBegin()
	k := softKey{cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2), auth: virtualwebauthn.NewAuthenticator()}
	k.auth.Options.UserHandle = []byte(opts.UserID)
	body := virtualwebauthn.CreateAttestationResponse(h.rp(), k.auth, k.cred, *opts)
	if rec := h.registerFinish(sessionID, label, body); rec.Code != http.StatusCreated {
		h.t.Fatalf("register finish: %d %s", rec.Code, rec.Body.String())
	}
	k.auth.AddCredential(k.cred)
	return k
}

// passkeyAssertion starts a login and returns the finish target and the signed assertion body.
func (h *mfaHarness) passkeyAssertion(k softKey, rp virtualwebauthn.RelyingParty) (target, body string) {
	h.t.Helper()
	rec := doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"`+testAdminUsername+`"}`, nil)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("login begin: %d %s", rec.Code, rec.Body.String())
	}
	var resp passkeyLoginBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		h.t.Fatal(err)
	}
	raw, _ := json.Marshal(resp.Options)
	opts, err := virtualwebauthn.ParseAssertionOptions(string(raw))
	if err != nil {
		h.t.Fatalf("parse assertion options: %v", err)
	}
	return "/api/v1/auth/passkey-login/finish?session_id=" + url.QueryEscape(resp.SessionID),
		virtualwebauthn.CreateAssertionResponse(rp, k.auth, k.cred, *opts)
}

func (h *mfaHarness) finishLogin(target, body string) *httptest.ResponseRecorder {
	return doJSON(h.t, h.rt, http.MethodPost, target, body, nil)
}

func hasSessionCookie(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}

// normalizeBody makes a JSON body comparable across engines: ids, secrets, options and
// timestamps are replaced by a marker, recovery code lists by their length.
func normalizeBody(t *testing.T, raw string) any {
	t.Helper()
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("response is not JSON: %q", raw)
	}
	return redact(v)
}

func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			switch k {
			case "id", "secret", "provisioning_uri", "session_id", "options", "created_at", "last_used_at":
				x[k] = "<redacted>"
			case "recovery_codes":
				if codes, ok := val.([]any); ok {
					x[k] = fmt.Sprintf("<%d codes>", len(codes))
				}
			default:
				x[k] = redact(val)
			}
		}
		return x
	case []any:
		for i := range x {
			x[i] = redact(x[i])
		}
		return x
	}
	return v
}
