package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/samlsso/samltest"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	samlTestBase   = "https://cp.example.test"
	samlTestIDP    = "https://idp.example.test/metadata"
	samlTestIDPSSO = "https://idp.example.test/sso"
)

type samlHarness struct {
	t      *testing.T
	rt     *Router
	db     *store.DB
	admin  *http.Cookie
	idp    *samltest.IDP
	connID string
}

func newSAMLHarness(t *testing.T, conn map[string]any) *samlHarness {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithOAuthSecrets(newFakeOAuthSecrets()))
	admin := loginTestSession(t, rt, db)
	if err := db.SetDashboardURL(context.Background(), samlTestBase); err != nil {
		t.Fatal(err)
	}
	h := &samlHarness{t: t, rt: rt, db: db, admin: admin, idp: samltest.NewIDP(t, samlTestIDP)}
	body := map[string]any{
		"name": "Okta", "enabled": true, "idp_entity_id": samlTestIDP, "idp_sso_url": samlTestIDPSSO,
		"idp_certificates": h.idp.CertPEM(), "jit_provisioning": true, "default_role_id": "role_viewer",
	}
	for k, v := range conn {
		body[k] = v
	}
	rec := h.admin2(http.MethodPost, "/api/v1/sso/saml/connections", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create connection: %d %s", rec.Code, rec.Body)
	}
	var res samlConnectionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	h.connID = res.ID
	return h
}

func (h *samlHarness) admin2(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	if body == nil {
		raw = nil
	}
	req := authedRequest(h.t, h.admin, method, path, string(raw))
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *samlHarness) target(requestID string) samltest.Target {
	return samltest.Target{
		SPEntityID: samlTestBase + samlPathPrefix + h.connID + "/metadata",
		ACSURL:     samlTestBase + samlPathPrefix + h.connID + "/acs",
		RequestID:  requestID,
	}
}

// start runs the login redirect and returns the stored request ID and flow cookie.
func (h *samlHarness) start() (string, *http.Cookie) {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, samlPathPrefix+h.connID+"/login", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), samlTestIDPSSO+"?") {
		h.t.Fatalf("login: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	var flow *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == samlFlowCookie {
			flow = c
		}
	}
	if flow == nil || !flow.Secure || !flow.HttpOnly || flow.SameSite != http.SameSiteNoneMode {
		h.t.Fatalf("flow cookie = %+v", flow)
	}
	var id string
	if err := h.db.QueryRowContext(context.Background(), `SELECT id FROM saml_authn_requests ORDER BY rowid DESC LIMIT 1`).Scan(&id); err != nil {
		h.t.Fatal(err)
	}
	return id, flow
}

func (h *samlHarness) post(flow *http.Cookie, samlResponse string) *httptest.ResponseRecorder {
	h.t.Helper()
	form := url.Values{"SAMLResponse": {samlResponse}}
	req := httptest.NewRequest(http.MethodPost, samlPathPrefix+h.connID+"/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://idp.example.test")
	req.TLS = &tls.ConnectionState{}
	if flow != nil {
		req.AddCookie(&http.Cookie{Name: flow.Name, Value: flow.Value}) //nolint:gosec // request cookie, attributes only apply to Set-Cookie
	}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (h *samlHarness) respond(requestID, nameID, email string, groups ...string) string {
	attrs := map[string][]string{"email": {email}, "displayName": {"Test User"}}
	if len(groups) > 0 {
		attrs["groups"] = groups
	}
	tg := h.target(requestID)
	return h.idp.Respond(h.t, h.idp.Assertion(tg, nameID, attrs, time.Now()), h.idp.DefaultOptions(tg))
}

func sessionFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func wantSAMLError(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login?oauth_error="+code {
		t.Fatalf("got %d %q, want redirect with %s", rec.Code, rec.Header().Get("Location"), code)
	}
	if sessionFrom(rec) != nil {
		t.Fatal("a refused sign-in set a session cookie")
	}
}

func TestSAMLSignInProvisionsWithMappedRole(t *testing.T) {
	h := newSAMLHarness(t, map[string]any{"sync_roles": true})
	if rec := h.admin2(http.MethodPut, "/api/v1/sso/group-mappings", map[string]any{"mappings": []map[string]any{{"group": "eng", "role_id": "role_operator", "priority": 5}}}); rec.Code != http.StatusOK {
		t.Fatalf("mappings: %d %s", rec.Code, rec.Body)
	}
	id, flow := h.start()
	rec := h.post(flow, h.respond(id, "nid-ada", "ada@example.com", "eng"))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != samlCompleteRedirect {
		t.Fatalf("acs: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	sess := sessionFrom(rec)
	if sess == nil {
		t.Fatal("no session cookie")
	}
	user, err := h.db.GetUserByEmail(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if role, _ := h.db.UserRoleID(context.Background(), user.ID); role != "role_operator" {
		t.Fatalf("role = %q", role)
	}
	check := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(check, authedRequest(t, sess, http.MethodGet, "/api/v1/auth/session", ""))
	if check.Code != http.StatusOK {
		t.Fatalf("session check: %d", check.Code)
	}

	id2, flow2 := h.start()
	if rec = h.post(flow2, h.respond(id2, "nid-ada", "ada@example.com")); sessionFrom(rec) == nil {
		t.Fatalf("fallback role sign-in refused: %s", rec.Header().Get("Location"))
	}
	if role, _ := h.db.UserRoleID(context.Background(), user.ID); role != "role_viewer" {
		t.Fatalf("leaving the group kept role %q", role)
	}

	update := map[string]any{"name": "Okta", "enabled": true, "idp_entity_id": samlTestIDP, "idp_sso_url": samlTestIDPSSO,
		"idp_certificates": h.idp.CertPEM(), "jit_provisioning": true, "sync_roles": true}
	if rec := h.admin2(http.MethodPut, "/api/v1/sso/saml/connections/"+h.connID, update); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	id3, flow3 := h.start()
	wantSAMLError(t, h.post(flow3, h.respond(id3, "nid-ada", "ada@example.com")), samlErrNoRole)
}

func TestSAMLRefusesReplay(t *testing.T) {
	h := newSAMLHarness(t, nil)
	id, flow := h.start()
	resp := h.respond(id, "nid-1", "one@example.com")
	if rec := h.post(flow, resp); sessionFrom(rec) == nil {
		t.Fatalf("first use refused: %s", rec.Header().Get("Location"))
	}
	wantSAMLError(t, h.post(flow, resp), samlErrInvalidState)

	id2, flow2 := h.start()
	tg := h.target(id2)
	a := h.idp.Assertion(tg, "nid-1", map[string][]string{"email": {"one@example.com"}}, time.Now())
	first := h.idp.Assertion(tg, "nid-1", nil, time.Now())
	a.ID = first.ID
	if rec := h.post(flow2, h.idp.Respond(t, a, h.idp.DefaultOptions(tg))); sessionFrom(rec) == nil {
		t.Fatalf("fresh assertion refused: %s", rec.Header().Get("Location"))
	}
	id3, flow3 := h.start()
	tg3 := h.target(id3)
	b := h.idp.Assertion(tg3, "nid-1", map[string][]string{"email": {"one@example.com"}}, time.Now())
	b.ID = a.ID
	wantSAMLError(t, h.post(flow3, h.idp.Respond(t, b, h.idp.DefaultOptions(tg3))), samlErrReplayed)
}

func TestSAMLRejections(t *testing.T) {
	h := newSAMLHarness(t, map[string]any{"jit_provisioning": false, "allowed_domains": []string{"example.com"}})
	forger := samltest.NewIDP(t, samlTestIDP)

	id, flow := h.start()
	tg := h.target(id)
	forged := forger.Respond(t, forger.Assertion(tg, "nid-x", map[string][]string{"email": {"x@example.com"}}, time.Now()), forger.DefaultOptions(tg))
	wantSAMLError(t, h.post(flow, forged), samlErrInvalid)

	id, flow = h.start()
	wantSAMLError(t, h.post(flow, h.respond(id, "nid-new", "new@example.com")), samlErrNotProvisioned)

	id, flow = h.start()
	wantSAMLError(t, h.post(flow, h.respond(id, "nid-evil", "evil@other.test")), samlErrDomain)

	id, _ = h.start()
	wantSAMLError(t, h.post(nil, h.respond(id, "nid-new", "new@example.com")), samlErrInvalidState)

	_, flow = h.start()
	wrong := h.respond("id-some-other-request", "nid-x", "x@example.com")
	wantSAMLError(t, h.post(flow, wrong), samlErrInvalid)

	existing := storeUserForTest(t, h.db, "known@example.com")
	id, flow = h.start()
	rec := h.post(flow, h.respond(id, "nid-known", "known@example.com"))
	if sessionFrom(rec) == nil {
		t.Fatalf("existing account refused: %s", rec.Header().Get("Location"))
	}
	if uid, err := h.db.SAMLIdentityUser(context.Background(), h.connID, "nid-known"); err != nil || uid != existing.ID {
		t.Fatalf("link = %q %v", uid, err)
	}

	plain := httptest.NewRequest(http.MethodGet, samlPathPrefix+h.connID+"/login", nil)
	plainRec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(plainRec, plain)
	wantSAMLError(t, plainRec, samlErrUnavailable)
}

func TestSAMLConnectionValidation(t *testing.T) {
	h := newSAMLHarness(t, nil)
	cases := map[string]map[string]any{
		"incomplete enabled": {"name": "B", "enabled": true, "idp_entity_id": samlTestIDP},
		"http sso url":       {"name": "C", "enabled": true, "idp_entity_id": samlTestIDP, "idp_sso_url": "http://idp/sso", "idp_certificates": h.idp.CertPEM()},
		"bad certificate":    {"name": "D", "idp_certificates": "not a cert"},
		"unknown role":       {"name": "E", "default_role_id": "role_nope"},
		"bad domain":         {"name": "F", "allowed_domains": []string{"not a domain"}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := h.admin2(http.MethodPost, "/api/v1/sso/saml/connections", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status %d %s", rec.Code, rec.Body)
			}
		})
	}
	rec := h.admin2(http.MethodPost, "/api/v1/sso/saml/connections", map[string]any{"name": "Draft"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("draft: %d %s", rec.Code, rec.Body)
	}
	var draft samlConnectionResource
	_ = json.Unmarshal(rec.Body.Bytes(), &draft)
	rec = h.admin2(http.MethodPost, "/api/v1/sso/saml/connections/"+draft.ID+"/metadata", map[string]any{"xml": h.idp.MetadataXML(samlTestIDPSSO)})
	var imported samlConnectionResource
	_ = json.Unmarshal(rec.Body.Bytes(), &imported)
	if rec.Code != http.StatusOK || imported.IDPEntityID != samlTestIDP || !imported.Configured || imported.Enabled {
		t.Fatalf("import: %d %+v", rec.Code, imported)
	}
	if imported.SPACSURL != samlTestBase+samlPathPrefix+draft.ID+"/acs" {
		t.Fatalf("acs = %q", imported.SPACSURL)
	}
	md := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(md, httptest.NewRequest(http.MethodGet, samlPathPrefix+draft.ID+"/metadata", nil))
	if md.Code != http.StatusOK || !strings.Contains(md.Body.String(), imported.SPACSURL) {
		t.Fatalf("metadata: %d", md.Code)
	}
}

func TestSSOEnforcementKeepsRootPassword(t *testing.T) {
	h := newSAMLHarness(t, nil)
	if rec := h.admin2(http.MethodPut, "/api/v1/sso/settings", map[string]any{"enforce_sso": true}); rec.Code != http.StatusOK {
		t.Fatalf("enforce: %d %s", rec.Code, rec.Body)
	}
	if err := h.db.SetDashboardURL(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	dev := storeUserWithAbilitiesForTest(t, h.db, "dev@example.com", []string{"read"})
	if !h.rt.ssoBlocksLocalSignIn(context.Background(), dev.ID) {
		t.Fatal("enforcement let a non-root user sign in locally")
	}
	root, err := h.db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	if h.rt.ssoBlocksLocalSignIn(context.Background(), root.ID) {
		t.Fatal("enforcement blocked the root break-glass sign-in")
	}
	_ = loginTestSession(t, h.rt, h.db)
}
