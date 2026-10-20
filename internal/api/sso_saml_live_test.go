package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

const liveSAMLEnv = "LEVELRAIL_LIVE_IDP"

func liveEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func skipUnlessLiveIDP(t *testing.T) {
	t.Helper()
	if testing.Short() || os.Getenv(liveSAMLEnv) != "1" {
		t.Skip("live IdP test: set " + liveSAMLEnv + "=1 and start the IdP containers")
	}
}

// liveIDP drives one real identity provider without a browser.
type liveIDP interface {
	label() string
	metadataXML(t *testing.T) []byte
	attributes() (email, name, groups string)
	register(t *testing.T, spEntityID, acsURL string, mode liveSignMode)
	login(t *testing.T, redirect, user, pass string) string
}

type liveSignMode string

const (
	liveSignAssertion liveSignMode = "assertion"
	liveSignResponse  liveSignMode = "response"
	liveSignNone      liveSignMode = "none"
)

var (
	liveFormRe  = regexp.MustCompile(`(?is)<form\b[^>]*>`)
	liveActRe   = regexp.MustCompile(`(?is)\baction\s*=\s*"([^"]*)"`)
	liveInputRe = regexp.MustCompile(`(?is)<input\b[^>]*>`)
	liveNameRe  = regexp.MustCompile(`(?is)\bname\s*=\s*"([^"]*)"`)
	liveValRe   = regexp.MustCompile(`(?is)\bvalue\s*=\s*"([^"]*)"`)
)

type liveForm struct {
	action string
	fields url.Values
}

func parseLiveForm(base *url.URL, body string) (liveForm, bool) {
	loc := liveFormRe.FindStringIndex(body)
	if loc == nil {
		return liveForm{}, false
	}
	am := liveActRe.FindStringSubmatch(body[loc[0]:loc[1]])
	action := base.String()
	if am != nil {
		ref, err := url.Parse(html.UnescapeString(am[1]))
		if err == nil {
			action = base.ResolveReference(ref).String()
		}
	}
	f := liveForm{action: action, fields: url.Values{}}
	for _, in := range liveInputRe.FindAllString(body[loc[1]:], -1) {
		nm := liveNameRe.FindStringSubmatch(in)
		if nm == nil {
			continue
		}
		v := ""
		if vm := liveValRe.FindStringSubmatch(in); vm != nil {
			v = html.UnescapeString(vm[1])
		}
		f.fields.Set(html.UnescapeString(nm[1]), v)
	}
	return f, true
}

func newLiveClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // local test IdP containers use self-signed certificates
	return &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: tr}
}

func liveGet(t *testing.T, c *http.Client, target string) (string, *url.URL) {
	t.Helper()
	resp, err := c.Get(target)
	if err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return string(b), resp.Request.URL
}

// liveBrowse follows an IdP sign-in page, fills the login form and returns the SAMLResponse it posts back.
func liveBrowse(t *testing.T, c *http.Client, start, user, pass string) string {
	t.Helper()
	body, cur := liveGet(t, c, start)
	for i := 0; i < 6; i++ {
		f, ok := parseLiveForm(cur, body)
		if !ok {
			t.Fatalf("no form on %s (step %d): %.300s", cur, i, body)
		}
		if v := f.fields.Get("SAMLResponse"); v != "" {
			return v
		}
		f.fields.Set("username", user)
		f.fields.Set("password", pass)
		resp, err := c.PostForm(f.action, f.fields)
		if err != nil {
			t.Fatalf("POST login form: %v", err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		body, cur = string(b), resp.Request.URL
	}
	t.Fatalf("no SAMLResponse after login: %.300s", body)
	return ""
}

type liveKeycloak struct {
	base, realm, token string
	client             *http.Client
}

func newLiveKeycloak(t *testing.T) *liveKeycloak {
	t.Helper()
	k := &liveKeycloak{base: liveEnvOr("LEVELRAIL_LIVE_KEYCLOAK_URL", "https://localhost:18443"), realm: "lr", client: newLiveClient()}
	resp, err := k.client.PostForm(k.base+"/realms/master/protocol/openid-connect/token", url.Values{
		"grant_type": {"password"}, "client_id": {"admin-cli"}, "username": {"admin"}, "password": {"admin"}})
	if err != nil {
		t.Fatalf("keycloak token: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		t.Fatalf("keycloak token decode: %v", err)
	}
	k.token = tok.AccessToken
	k.client = newLiveClient()
	k.setupRealm(t)
	return k
}

func (k *liveKeycloak) admin(t *testing.T, method, path string, body any) (int, []byte, http.Header) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, k.base+"/admin/realms"+path, rd)
	req.Header.Set("Authorization", "Bearer "+k.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := k.client.Do(req)
	if err != nil {
		t.Fatalf("keycloak %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}

func (k *liveKeycloak) setupRealm(t *testing.T) {
	t.Helper()
	k.admin(t, http.MethodDelete, "/"+k.realm, nil)
	if code, b, _ := k.admin(t, http.MethodPost, "", map[string]any{"realm": k.realm, "enabled": true, "sslRequired": "none"}); code != http.StatusCreated {
		t.Fatalf("create realm: %d %s", code, b)
	}
	_, _, h := k.admin(t, http.MethodPost, "/"+k.realm+"/groups", map[string]any{"name": "Engineering"})
	_ = h
	users := []struct{ name, email, first string }{{"ada", "ada@example.com", "Ada"}, {"grace", "grace@example.com", "Grace"}}
	for _, u := range users {
		code, b, hdr := k.admin(t, http.MethodPost, "/"+k.realm+"/users", map[string]any{
			"username": u.name, "email": u.email, "firstName": u.first, "lastName": "Tester", "enabled": true, "emailVerified": true,
			"credentials": []map[string]any{{"type": "password", "value": u.name + "-pass", "temporary": false}},
		})
		if code != http.StatusCreated {
			t.Fatalf("create user %s: %d %s", u.name, code, b)
		}
		if u.name == "ada" {
			uid := filepath.Base(hdr.Get("Location"))
			_, gb, _ := k.admin(t, http.MethodGet, "/"+k.realm+"/groups?search=Engineering", nil)
			var groups []struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(gb, &groups)
			if len(groups) == 0 {
				t.Fatal("group not found")
			}
			if c, b, _ := k.admin(t, http.MethodPut, "/"+k.realm+"/users/"+uid+"/groups/"+groups[0].ID, nil); c != http.StatusNoContent {
				t.Fatalf("join group: %d %s", c, b)
			}
		}
	}
}

func (k *liveKeycloak) label() string                        { return "keycloak" }
func (k *liveKeycloak) attributes() (string, string, string) { return "email", "displayName", "groups" }

func (k *liveKeycloak) metadataXML(t *testing.T) []byte {
	t.Helper()
	b, _ := liveGet(t, k.client, k.base+"/realms/"+k.realm+"/protocol/saml/descriptor")
	return []byte(b)
}

func (k *liveKeycloak) register(t *testing.T, spEntityID, acsURL string, mode liveSignMode) {
	t.Helper()
	attr := map[string]string{
		"saml.assertion.signature": fmt.Sprint(mode == liveSignAssertion), "saml.server.signature": fmt.Sprint(mode == liveSignResponse),
		"saml.client.signature": "false", "saml_name_id_format": "email", "saml.force.post.binding": "true",
		"saml_assertion_consumer_url_post": acsURL, "saml_idp_initiated_sso_url_name": "lr-init", "saml_idp_initiated_sso_relay_state": "",
	}
	mapper := func(name, kind string, cfg map[string]string) map[string]any {
		cfg["attribute.nameformat"] = "Basic"
		return map[string]any{"name": name, "protocol": "saml", "protocolMapper": kind, "config": cfg}
	}
	body := map[string]any{
		"clientId": spEntityID, "protocol": "saml", "enabled": true, "redirectUris": []string{"*"}, "attributes": attr,
		"protocolMappers": []map[string]any{
			mapper("email", "saml-user-property-mapper", map[string]string{"user.attribute": "email", "attribute.name": "email"}),
			mapper("displayName", "saml-user-property-mapper", map[string]string{"user.attribute": "firstName", "attribute.name": "displayName"}),
			mapper("groups", "saml-group-membership-mapper", map[string]string{"attribute.name": "groups", "full.path": "false", "single": "false"}),
		},
	}
	code, b, hdr := k.admin(t, http.MethodPost, "/"+k.realm+"/clients", body)
	if code == http.StatusConflict {
		_, lb, _ := k.admin(t, http.MethodGet, "/"+k.realm+"/clients?clientId="+url.QueryEscape(spEntityID), nil)
		var cs []struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(lb, &cs)
		if len(cs) == 1 {
			k.admin(t, http.MethodDelete, "/"+k.realm+"/clients/"+cs[0].ID, nil)
			code, b, hdr = k.admin(t, http.MethodPost, "/"+k.realm+"/clients", body)
		}
	}
	if code != http.StatusCreated {
		t.Fatalf("create client: %d %s", code, b)
	}
	_ = hdr
}

func (k *liveKeycloak) login(t *testing.T, redirect, user, pass string) string {
	t.Helper()
	return liveBrowse(t, newLiveClient(), redirect, user, pass)
}

func (k *liveKeycloak) idpInitiated(t *testing.T, user, pass string) string {
	t.Helper()
	return liveBrowse(t, newLiveClient(), k.base+"/realms/"+k.realm+"/protocol/saml/clients/lr-init", user, pass)
}

type liveSSP struct {
	base, container string
	client          *http.Client
}

func newLiveSSP() *liveSSP {
	return &liveSSP{base: liveEnvOr("LEVELRAIL_LIVE_SSP_URL", "https://localhost:18182"), container: liveEnvOr("LEVELRAIL_LIVE_SSP_CONTAINER", "lr-sp"), client: newLiveClient()}
}

func (s *liveSSP) label() string { return "simplesamlphp" }
func (s *liveSSP) attributes() (string, string, string) {
	return "email", "uid", "eduPersonAffiliation"
}

func (s *liveSSP) metadataXML(t *testing.T) []byte {
	t.Helper()
	b, _ := liveGet(t, s.client, s.base+"/simplesaml/saml2/idp/metadata.php")
	return []byte(b)
}

func (s *liveSSP) register(t *testing.T, spEntityID, acsURL string, mode liveSignMode) {
	t.Helper()
	php := fmt.Sprintf(`<?php
$metadata['%s'] = array(
  'AssertionConsumerService' => '%s',
  'NameIDFormat' => 'urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress',
  'simplesaml.nameidattribute' => 'email',
  'saml20.sign.assertion' => %t,
  'saml20.sign.response' => %t,
);
`, spEntityID, acsURL, mode == liveSignAssertion, mode == liveSignResponse)
	f := filepath.Join(t.TempDir(), "saml20-sp-remote.php")
	if err := os.WriteFile(f, []byte(php), 0o644); err != nil { //nolint:gosec // the IdP container user must read the file
		t.Fatal(err)
	}
	out, err := exec.Command("docker", "cp", f, s.container+":/var/www/simplesamlphp/metadata/saml20-sp-remote.php").CombinedOutput() //nolint:gosec // test helper, container name from a test env var
	if err != nil {
		t.Fatalf("docker cp: %v %s", err, out)
	}
}

func (s *liveSSP) login(t *testing.T, redirect, user, pass string) string {
	t.Helper()
	return liveBrowse(t, newLiveClient(), redirect, user, pass)
}

func newLiveHarness(t *testing.T, idp liveIDP, mode liveSignMode, conn map[string]any) *samlHarness {
	t.Helper()
	db := openTestDB(t)
	liveLogs.Reset()
	rt := NewRouter(slog.New(slog.NewTextHandler(&liveLogs, nil)), testBrand(), db, WithOAuthSecrets(newFakeOAuthSecrets()))
	admin := loginTestSession(t, rt, db)
	if err := db.SetDashboardURL(context.Background(), samlTestBase); err != nil {
		t.Fatal(err)
	}
	h := &samlHarness{t: t, rt: rt, db: db, admin: admin}
	meta, err := authengine.ParseSAMLIdPMetadata(idp.metadataXML(t), time.Now())
	if err != nil {
		t.Fatalf("parse %s metadata: %v", idp.label(), err)
	}
	email, name, groups := idp.attributes()
	body := map[string]any{
		"name": idp.label(), "enabled": true, "idp_entity_id": meta.EntityID, "idp_sso_url": meta.SSOURL,
		"idp_certificates": meta.CertPEM, "jit_provisioning": true, "default_role_id": "role_viewer",
		"email_attribute": email, "name_attribute": name, "groups_attribute": groups,
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
	idp.register(t, res.SPEntityID, res.SPACSURL, mode)
	return h
}

func (h *samlHarness) liveStart() (redirect string, flow *http.Cookie) {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, samlPathPrefix+h.connID+"/login", nil)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		h.t.Fatalf("login: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == samlFlowCookie {
			flow = c
		}
	}
	if flow == nil {
		h.t.Fatal("no flow cookie")
	}
	return rec.Header().Get("Location"), flow
}

func (h *samlHarness) liveSignIn(idp liveIDP, user, pass string) (*http.Cookie, string) {
	h.t.Helper()
	redirect, flow := h.liveStart()
	return flow, idp.login(h.t, redirect, user, pass)
}

var liveLogs syncBuffer

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func wantLiveReason(t *testing.T, substr string) {
	t.Helper()
	logs := liveLogs.String()
	i := strings.LastIndex(logs, "saml sign-in refused")
	if i < 0 || !strings.Contains(logs[i:], substr) {
		t.Fatalf("refusal reason does not mention %q: %.400s", substr, logs[max(i, 0):])
	}
}

func tamperSAMLResponse(t *testing.T, resp, from, to string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(resp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(from)) {
		t.Fatalf("response does not contain %q", from)
	}
	return base64.StdEncoding.EncodeToString(bytes.ReplaceAll(raw, []byte(from), []byte(to)))
}

func TestLiveSAMLAgainstRealIdPs(t *testing.T) {
	skipUnlessLiveIDP(t)
	idps := []liveIDP{newLiveKeycloak(t), newLiveSSP()}
	users := map[string][3]string{
		"keycloak":      {"ada", "ada-pass", "grace-pass"},
		"simplesamlphp": {"user1", "user1pass", "user2pass"},
	}
	emails := map[string][2]string{
		"keycloak":      {"ada@example.com", "grace@example.com"},
		"simplesamlphp": {"user1@example.com", "user2@example.com"},
	}
	for _, idp := range idps {
		idp := idp
		u, em := users[idp.label()], emails[idp.label()]
		t.Run(idp.label(), func(t *testing.T) {
			t.Run("signed assertion signs in with attributes and mapped role", func(t *testing.T) {
				h := newLiveHarness(t, idp, liveSignAssertion, map[string]any{"sync_roles": true})
				group := map[string]string{"keycloak": "Engineering", "simplesamlphp": "group1"}[idp.label()]
				if rec := h.admin2(http.MethodPut, "/api/v1/sso/group-mappings", map[string]any{"mappings": []map[string]any{{"group": group, "role_id": "role_operator", "priority": 5}}}); rec.Code != http.StatusOK {
					t.Fatalf("mappings: %d %s", rec.Code, rec.Body)
				}
				flow, resp := h.liveSignIn(idp, u[0], u[1])
				wantSAMLSession(t, h, h.post(flow, resp))
				user, err := h.db.GetUserByEmail(context.Background(), em[0])
				if err != nil {
					t.Fatalf("user not provisioned: %v", err)
				}
				if role, _ := h.db.UserRoleID(context.Background(), user.ID); role != "role_operator" {
					t.Fatalf("role = %q, want role_operator", role)
				}
				t.Run("second sign-in links the same account", func(t *testing.T) {
					flow, resp := h.liveSignIn(idp, u[0], u[1])
					wantSAMLSession(t, h, h.post(flow, resp))
					again, _ := h.db.GetUserByEmail(context.Background(), em[0])
					if again.ID != user.ID {
						t.Fatalf("second sign-in made account %s, want %s", again.ID, user.ID)
					}
				})
			})

			t.Run("jit off refuses an unknown user, jit on creates the viewer", func(t *testing.T) {
				h := newLiveHarness(t, idp, liveSignAssertion, map[string]any{"jit_provisioning": false})
				flow, resp := h.liveSignIn(idp, u[0], u[1])
				wantSAMLError(t, h.post(flow, resp), samlErrNotProvisioned)
				storeUserForTest(t, h.db, em[0])
				flow, resp = h.liveSignIn(idp, u[0], u[1])
				wantSAMLSession(t, h, h.post(flow, resp))
			})

			t.Run("tampered response is refused", func(t *testing.T) {
				h := newLiveHarness(t, idp, liveSignAssertion, nil)
				flow, resp := h.liveSignIn(idp, u[0], u[1])
				wantSAMLError(t, h.post(flow, tamperSAMLResponse(t, resp, em[0], "eve@example.com")), samlErrInvalid)
				wantLiveReason(t, "Signature could not be verified")
			})

			t.Run("replayed response is refused", func(t *testing.T) {
				h := newLiveHarness(t, idp, liveSignAssertion, nil)
				flow, resp := h.liveSignIn(idp, u[0], u[1])
				wantSAMLSession(t, h, h.post(flow, resp))
				wantSAMLError(t, h.post(flow, resp), samlErrInvalidState)
				_, flow2 := h.liveStart()
				wantSAMLError(t, h.post(flow2, resp), samlErrInvalid)
				wantLiveReason(t, "InResponseTo")
			})

			t.Run("response without a flow cookie is refused", func(t *testing.T) {
				h := newLiveHarness(t, idp, liveSignAssertion, nil)
				_, resp := h.liveSignIn(idp, u[0], u[1])
				wantSAMLError(t, h.post(nil, resp), samlErrInvalidState)
			})

			t.Run("response for another connection is refused", func(t *testing.T) {
				a := newLiveHarness(t, idp, liveSignAssertion, nil)
				_, resp := a.liveSignIn(idp, u[0], u[1])
				b := newLiveHarness(t, idp, liveSignAssertion, nil)
				_, flowB := b.liveStart()
				wantSAMLError(t, b.post(flowB, resp), samlErrInvalid)
				wantLiveReason(t, "Destination")
			})

			t.Run("assertion that is not signed is refused", func(t *testing.T) {
				want := map[liveSignMode]string{liveSignResponse: "assertion not signed", liveSignNone: "signature element not present"}
				for _, mode := range []liveSignMode{liveSignResponse, liveSignNone} {
					h := newLiveHarness(t, idp, mode, nil)
					flow, resp := h.liveSignIn(idp, u[0], u[1])
					wantSAMLError(t, h.post(flow, resp), samlErrInvalid)
					wantLiveReason(t, want[mode])
					if _, err := h.db.GetUserByEmail(context.Background(), em[0]); err == nil {
						t.Fatalf("refused %s-signed sign-in left an account behind", mode)
					}
				}
			})
		})
	}

	t.Run("keycloak imports the service provider metadata", func(t *testing.T) {
		kc := idps[0].(*liveKeycloak)
		h := newLiveHarness(t, kc, liveSignAssertion, nil)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, samlPathPrefix+h.connID+"/metadata", nil)
		req.TLS = &tls.ConnectionState{}
		h.rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("metadata: %d %s", rec.Code, rec.Body)
		}
		hreq, _ := http.NewRequest(http.MethodPost, kc.base+"/admin/realms/"+kc.realm+"/client-description-converter", bytes.NewReader(rec.Body.Bytes()))
		hreq.Header.Set("Authorization", "Bearer "+kc.token)
		hreq.Header.Set("Content-Type", "application/xml")
		resp, err := kc.client.Do(hreq)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var rep struct {
			ClientID     string   `json:"clientId"`
			RedirectURIs []string `json:"redirectUris"`
		}
		raw, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(raw, &rep); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("convert: %d %s", resp.StatusCode, raw)
		}
		want := samlTestBase + samlPathPrefix + h.connID + "/metadata"
		if rep.ClientID != want || len(rep.RedirectURIs) == 0 || rep.RedirectURIs[0] != samlTestBase+samlPathPrefix+h.connID+"/acs" {
			t.Fatalf("keycloak read client %q redirects %v, want %q", rep.ClientID, rep.RedirectURIs, want)
		}
	})

	t.Run("keycloak idp initiated is refused", func(t *testing.T) {
		kc := idps[0].(*liveKeycloak)
		h := newLiveHarness(t, kc, liveSignAssertion, nil)
		resp := kc.idpInitiated(t, "ada", "ada-pass")
		wantSAMLError(t, h.post(nil, resp), samlErrInvalidState)
		_, flow := h.liveStart()
		wantSAMLError(t, h.post(flow, resp), samlErrInvalid)
	})
}
