package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	testGateSecret = "0123456789abcdef0123456789abcdef"
	testGateHost   = "app.example.com"
	testGateCP     = "cp.example.com"
)

type authGateFixture struct {
	t       *testing.T
	rt      *Router
	db      *store.DB
	cookie  *http.Cookie
	adminID string
}

func newAuthGateFixture(t *testing.T, allow string) *authGateFixture {
	t.Helper()
	rt, db := newTestRouter(t)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "img:v1", Port: 8080, Domains: []string{testGateHost, "other.example.com"},
	}); err != nil {
		t.Fatal(err)
	}
	cookie := loginTestSession(t, rt, db)
	settings, err := db.GetIngressSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.PrimaryDomain = testGateCP
	if err := db.UpdateIngressSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	rt.SetGate(testGateSecret, DefaultGateSettings())
	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatal(err)
	}
	f := &authGateFixture{t: t, rt: rt, db: db, cookie: cookie, adminID: admin.ID}
	if allow == "admin" {
		allow = `[{"kind":"user","subject":"` + admin.ID + `"}]`
	}
	for _, host := range []string{testGateHost, "other.example.com"} {
		f.putGate(host, allow, http.StatusOK)
	}
	return f
}

func (f *authGateFixture) putGate(host, allow string, want int) {
	f.t.Helper()
	rec := httptest.NewRecorder()
	body := `{"enabled":true,"provider":"builtin","allow":` + allow + `}`
	f.rt.Handler().ServeHTTP(rec, authedRequest(f.t, f.cookie, http.MethodPut, "/api/v1/apps/web/domains/"+host+"/auth-gate", body))
	if rec.Code != want {
		f.t.Fatalf("PUT gate %s = %d %s", host, rec.Code, rec.Body.String())
	}
}

// check calls the check endpoint the way the ingress subrequest does.
func (f *authGateFixture) check(host, uri string, hdr map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gate/check", nil)
	req.Header.Set(ingress.GateHeaderSecret, testGateSecret)
	req.Header.Set(ingress.GateHeaderHost, host)
	req.Header.Set(ingress.GateHeaderProto, "https")
	req.Header.Set(ingress.GateHeaderClientIP, "203.0.113.7")
	req.Header.Set("X-Forwarded-Uri", uri)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (f *authGateFixture) login(rawQuery string, withSession bool) *httptest.ResponseRecorder {
	f.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/gate/login?"+rawQuery, nil)
	if withSession {
		req.AddCookie(f.cookie)
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// signIn walks the whole handoff and returns the gate session cookie.
func (f *authGateFixture) signIn(host, uri string) *http.Cookie {
	f.t.Helper()
	token, state := f.mintToken(host, uri)
	sep := "?"
	if strings.Contains(uri, "?") {
		sep = "&"
	}
	rec := f.check(host, uri+sep+gateTokenParam+"="+token, nil, state)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != uri {
		f.t.Fatalf("exchange = %d Location %q body %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	sess := cookieNamed(rec, "__Host-gate_session")
	if sess == nil {
		f.t.Fatal("exchange set no gate session cookie")
	}
	return sess
}

// mintToken returns a handoff token for host plus the state cookie it is bound to.
func (f *authGateFixture) mintToken(host, uri string) (string, *http.Cookie) {
	f.t.Helper()
	rec := f.check(host, uri, nil)
	if rec.Code != http.StatusFound {
		f.t.Fatalf("anonymous check = %d %s", rec.Code, rec.Body.String())
	}
	state := cookieNamed(rec, "__Host-gate_state")
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil || state == nil {
		f.t.Fatalf("login redirect %q, state %v", rec.Header().Get("Location"), state)
	}
	rec = f.login(loc.RawQuery, true)
	if rec.Code != http.StatusFound {
		f.t.Fatalf("gate login = %d %s", rec.Code, rec.Body.String())
	}
	back, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		f.t.Fatal(err)
	}
	return back.Query().Get(gateTokenParam), state
}

func TestGateCheck_RequiresSecret(t *testing.T) {
	f := newAuthGateFixture(t, "admin")
	for name, secret := range map[string]string{"missing": "", "wrong": "nope", "prefix": testGateSecret[:10]} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/gate/check", nil)
			if secret != "" {
				req.Header.Set(ingress.GateHeaderSecret, secret)
			}
			req.Header.Set(ingress.GateHeaderHost, testGateHost)
			req.Header.Set(ingress.GateHeaderProto, "https")
			req.Header.Set("X-Forwarded-Uri", "/")
			rec := httptest.NewRecorder()
			f.rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound || rec.Header().Get("X-User") != "" {
				t.Errorf("status = %d, X-User %q", rec.Code, rec.Header().Get("X-User"))
			}
		})
	}
}

func TestGateCheck_AnonymousRedirectAndXHR(t *testing.T) {
	f := newAuthGateFixture(t, "admin")
	rec := f.check(testGateHost, "/orders?id=7", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Scheme != "https" || loc.Host != testGateCP || loc.Path != "/gate/login" ||
		loc.Query().Get("rd") != "https://app.example.com/orders?id=7" || !validGateTokenShape(loc.Query().Get("state")) {
		t.Errorf("Location = %s", loc)
	}
	c := cookieNamed(rec, "__Host-gate_state")
	if c == nil || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
		t.Errorf("state cookie = %+v", c)
	}

	for name, hdr := range map[string]map[string]string{
		"accept json":      {"Accept": "application/json"},
		"x-requested-with": {"X-Requested-With": "XMLHttpRequest"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := f.check(testGateHost, "/api/x", hdr)
			var body gateLoginRequired
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			if rec.Code != http.StatusUnauthorized || !strings.HasPrefix(body.LoginURL, "https://"+testGateCP+"/gate/login?") || rec.Header().Get("Location") != "" {
				t.Errorf("xhr = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGateFlow_SignInAllowAndCookie(t *testing.T) {
	f := newAuthGateFixture(t, "admin")
	token, state := f.mintToken(testGateHost, "/orders?id=7")
	rec := f.check(testGateHost, "/orders?id=7&"+gateTokenParam+"="+token, nil, state)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/orders?id=7" {
		t.Fatalf("exchange = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	sess := cookieNamed(rec, "__Host-gate_session")
	if sess == nil || !sess.HttpOnly || !sess.Secure || sess.SameSite != http.SameSiteLaxMode || sess.Path != "/" || sess.Domain != "" || sess.MaxAge != int((12*time.Hour).Seconds()) {
		t.Fatalf("session cookie = %+v", sess)
	}
	if c := cookieNamed(rec, "__Host-gate_state"); c == nil || c.MaxAge >= 0 {
		t.Errorf("state cookie not cleared: %+v", c)
	}

	rec = f.check(testGateHost, "/orders", map[string]string{"X-User": "mallory"}, sess)
	if rec.Code != http.StatusOK || rec.Header().Get("X-User") != testAdminUsername || rec.Header().Get("X-Email") != testAdminUsername {
		t.Fatalf("allowed check = %d X-User %q", rec.Code, rec.Header().Get("X-User"))
	}

	rec = f.check(testGateHost, "/orders?"+gateTokenParam+"="+token, nil, state)
	if rec.Code != http.StatusForbidden || cookieNamed(rec, "__Host-gate_session") != nil {
		t.Errorf("replayed token = %d", rec.Code)
	}
	rec = f.check("other.example.com", "/", nil, sess)
	if rec.Code != http.StatusFound {
		t.Errorf("session cookie on another host = %d, want login redirect", rec.Code)
	}
}

func TestGateExchange_Rejections(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *authGateFixture) (host, token string, state *http.Cookie)
	}{
		{"garbage token", func(f *authGateFixture) (string, string, *http.Cookie) {
			_, state := f.mintToken(testGateHost, "/")
			return testGateHost, "not-a-token", state
		}},
		{"unknown token", func(f *authGateFixture) (string, string, *http.Cookie) {
			_, state := f.mintToken(testGateHost, "/")
			tok, _, _ := newGateSecretToken()
			return testGateHost, tok, state
		}},
		{"other host", func(f *authGateFixture) (string, string, *http.Cookie) {
			tok, state := f.mintToken(testGateHost, "/")
			return "other.example.com", tok, state
		}},
		{"missing state cookie", func(f *authGateFixture) (string, string, *http.Cookie) {
			tok, _ := f.mintToken(testGateHost, "/")
			return testGateHost, tok, nil
		}},
		{"wrong state cookie", func(f *authGateFixture) (string, string, *http.Cookie) {
			tok, _ := f.mintToken(testGateHost, "/")
			other, _, _ := newGateSecretToken()
			return testGateHost, tok, gateStateCookie(other)
		}},
		{"expired", func(f *authGateFixture) (string, string, *http.Cookie) {
			tok, hash, _ := newGateSecretToken()
			state, stateHash, _ := newGateSecretToken()
			sid := f.dashboardSessionID()
			if err := f.db.CreateGateToken(context.Background(), store.GateToken{TokenHash: hash, Domain: testGateHost, UserID: f.adminID, ParentSessionID: sid, StateHash: stateHash, ExpiresAt: time.Now().Add(-time.Second)}); err != nil {
				f.t.Fatal(err)
			}
			_ = state
			return testGateHost, tok, gateStateCookie(state)
		}},
		{"dashboard signed out", func(f *authGateFixture) (string, string, *http.Cookie) {
			tok, state := f.mintToken(testGateHost, "/")
			f.rt.sessions.revoke(f.cookie.Value)
			return testGateHost, tok, state
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthGateFixture(t, "admin")
			host, token, state := tt.setup(f)
			var cookies []*http.Cookie
			if state != nil {
				cookies = append(cookies, state)
			}
			rec := f.check(host, "/?"+gateTokenParam+"="+url.QueryEscape(token), nil, cookies...)
			if rec.Code != http.StatusForbidden || cookieNamed(rec, "__Host-gate_session") != nil || rec.Header().Get("X-User") != "" {
				t.Errorf("status = %d, cookies %v", rec.Code, rec.Result().Cookies())
			}
		})
	}
}

func gateStateCookie(v string) *http.Cookie {
	return &http.Cookie{Name: "__Host-gate_state", Value: v, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

func (f *authGateFixture) dashboardSessionID() string {
	f.t.Helper()
	info, ok := f.rt.sessions.lib.Lookup(context.Background(), f.cookie.Value)
	if !ok {
		f.t.Fatal("no dashboard session")
	}
	return info.SessionID
}

func TestGateCheck_RevocationAndAccessRemoval(t *testing.T) {
	tests := []struct {
		name   string
		revoke func(f *authGateFixture)
		want   int
	}{
		{"admin signs everyone out", func(f *authGateFixture) {
			rec := httptest.NewRecorder()
			f.rt.Handler().ServeHTTP(rec, authedRequest(f.t, f.cookie, http.MethodDelete, "/api/v1/apps/web/domains/"+testGateHost+"/auth-gate/sessions", ""))
			if rec.Code != http.StatusOK {
				f.t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
			}
		}, http.StatusFound},
		{"dashboard sign out", func(f *authGateFixture) { f.rt.sessions.revoke(f.cookie.Value) }, http.StatusFound},
		{"access removed", func(f *authGateFixture) {
			other := storeUserForTest(f.t, f.db, "other@example.com")
			f.putGate(testGateHost, `[{"kind":"user","subject":"`+other.ID+`"}]`, http.StatusOK)
		}, http.StatusForbidden},
		{"all rules removed", func(f *authGateFixture) { f.putGate(testGateHost, `[]`, http.StatusOK) }, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAuthGateFixture(t, "admin")
			sess := f.signIn(testGateHost, "/")
			tt.revoke(f)
			rec := f.check(testGateHost, "/", nil, sess)
			if rec.Code != tt.want || rec.Header().Get("X-User") != "" {
				t.Errorf("after revoke = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestGateLogin_FailClosedAndDenied(t *testing.T) {
	f := newAuthGateFixture(t, "[]")
	state, _, _ := newGateSecretToken()
	q := url.Values{"rd": {"https://app.example.com/"}, "state": {state}}.Encode()
	rec := f.login(q, true)
	if rec.Code != http.StatusForbidden || rec.Header().Get("Location") != "" || !strings.Contains(rec.Body.String(), "Access denied") {
		t.Errorf("no rules = %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), testBrand().Name) {
		t.Error("gate page does not use the brand name")
	}
	rec = f.login(q, false)
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/login?redirect=%2Fgate%2Flogin%3F") {
		t.Errorf("signed out = %d %q", rec.Code, loc)
	}
}

func TestValidateGateRedirect(t *testing.T) {
	f := newAuthGateFixture(t, "admin")
	if _, err := f.db.ExecContext(context.Background(), `UPDATE domain_auth_gate SET enabled = 0 WHERE domain = 'other.example.com'`); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		raw, want string
		allowHTTP bool
	}{
		{raw: "https://app.example.com/a?b=1", want: "https://app.example.com/a?b=1"},
		{raw: "https://APP.example.com", want: "https://app.example.com/"},
		{raw: "https://app.example.com:8443//x#frag", want: "https://app.example.com:8443/x"},
		{raw: "https://app.example.com/?_gate_token=old&k=v", want: "https://app.example.com/?k=v"},
		{raw: "http://app.example.com/"},
		{raw: "http://app.example.com/", want: "http://app.example.com/", allowHTTP: true},
		{raw: "https://evil.com/"},
		{raw: "https://app.example.com.evil.com/"},
		{raw: "https://app.example.com@evil.com/"},
		{raw: "https://evil.com@app.example.com/"},
		{raw: "https://evil.com\\@app.example.com/"},
		{raw: "//app.example.com/"},
		{raw: "/relative"},
		{raw: "https:///app.example.com"},
		{raw: "javascript://app.example.com/%0aalert(1)"},
		{raw: "https://app.example.com/\tx"},
		{raw: "https://other.example.com/"},
		{raw: "https://[::1]/"},
		{raw: ""},
		{raw: "https://app.example.com/" + strings.Repeat("a", maxGateRedirectLen)},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			f.rt.gateSettings.AllowHTTP = tt.allowHTTP
			got, _, err := f.rt.validateGateRedirect(context.Background(), tt.raw)
			if tt.want == "" {
				if err == nil {
					t.Errorf("accepted as %q", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestParseGateRequest(t *testing.T) {
	tests := []struct {
		host, proto, uri string
		wantURI, token   string
		ok               bool
	}{
		{"app.example.com", "https", "/a?x=1&_gate_token=T&y=2", "/a?x=1&y=2", "T", true},
		{"App.Example.com:443", "https", "//evil.com/p", "/evil.com/p", "", true},
		{"app.example.com", "https", "/%5Cevil.com", "/%5Cevil.com", "", true},
		{"app.example.com", "ftp", "/", "", "", false},
		{"bad host", "https", "/", "", "", false},
		{"user@app.example.com", "https", "/", "", "", false},
		{"app.example.com", "https", "relative", "", "", false},
		{"app.example.com", "https", "/a\nb", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.host+tt.uri, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/gate/check", nil)
			r.Header.Set(ingress.GateHeaderHost, tt.host)
			r.Header.Set(ingress.GateHeaderProto, tt.proto)
			r.Header.Set("X-Forwarded-Uri", tt.uri)
			got, err := parseGateRequest(r)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok %v", err, tt.ok)
			}
			if tt.ok && (got.uri != tt.wantURI || got.token != tt.token || got.host != "app.example.com") {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestGateEvents_CoalescedAndListed(t *testing.T) {
	f := newAuthGateFixture(t, "admin")
	sess := f.signIn(testGateHost, "/")
	for i := 0; i < 3; i++ {
		if rec := f.check(testGateHost, "/", nil, sess); rec.Code != http.StatusOK {
			t.Fatalf("check = %d", rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookie, http.MethodGet, "/api/v1/apps/web/domains/"+testGateHost+"/auth-gate/events?since=1h", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("events = %d %s", rec.Code, rec.Body.String())
	}
	var res authGateEventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, e := range res.Events {
		seen[e.Decision] += e.Count
		if e.Decision == store.GateDecisionAllowed && (e.UserEmail != testAdminUsername || e.IP != "203.0.113.7") {
			t.Errorf("allowed event = %+v", e)
		}
	}
	if seen[store.GateDecisionAllowed] != 3 || seen[store.GateDecisionTokenExchange] != 1 || seen[store.GateDecisionTokenIssued] != 1 || seen[store.GateDecisionLoginRedirect] != 1 {
		t.Errorf("decisions = %v", seen)
	}
	allowedRows := 0
	for _, e := range res.Events {
		if e.Decision == store.GateDecisionAllowed {
			allowedRows++
		}
	}
	if allowedRows != 1 {
		t.Errorf("allowed rows = %d, want 1 coalesced row", allowedRows)
	}
}

func TestGateSettingsFromEnv(t *testing.T) {
	env := map[string]string{EnvGateSessionTTL: "2h", EnvGateEventRetention: "7d", EnvGateTokenTTL: "1h", EnvGateAllowHTTP: "true"}
	s, err := GateSettingsFromEnv(func(k string) string { return env[k] })
	if err != nil || s.SessionTTL != 2*time.Hour || s.EventRetention != 7*24*time.Hour || s.TokenTTL != maxGateTokenTTL || !s.AllowHTTP {
		t.Errorf("settings = %+v, %v", s, err)
	}
	s, err = GateSettingsFromEnv(func(k string) string { return map[string]string{EnvGateSessionTTL: "-1h"}[k] })
	if err == nil || s.SessionTTL != defaultGateSessionTTL {
		t.Errorf("bad value = %+v, %v", s, err)
	}
}

func TestSetDomainAuthGate_BuiltinValidation(t *testing.T) {
	f := newAuthGateFixture(t, "admin")
	f.putGate(testGateHost, `[{"kind":"user","subject":"nobody@example.com"}]`, http.StatusBadRequest)
	f.putGate(testGateHost, `[{"kind":"group","subject":"x"}]`, http.StatusBadRequest)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookie, http.MethodPut, "/api/v1/apps/web/domains/"+testGateHost+"/auth-gate", `{"enabled":true,"auth_url":"http://auth","allow":[{"kind":"user","subject":"x"}]}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("external with allow = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, f.cookie, http.MethodGet, "/api/v1/apps/web/domains/"+testGateHost+"/auth-gate", ""))
	var got domainAuthGateResource
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Provider != store.GateProviderBuiltin || len(got.Allow) != 1 || got.Allow[0].Label != testAdminUsername || !got.BuiltinAvailable {
		t.Errorf("get = %+v", got)
	}
}
