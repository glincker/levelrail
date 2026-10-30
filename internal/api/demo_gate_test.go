package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/store"
)

const demoTestVisitorID = "user_demo_visitor"

type demoTestEnv struct {
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
}

func newDemoTestEnv(t *testing.T) *demoTestEnv {
	t.Helper()
	db := openTestDB(t)
	visitor := store.User{ID: demoTestVisitorID, Email: "demo", DisplayName: "Alex Rivera", Abilities: []string{AbilityRead}}
	if err := db.CreateUser(t.Context(), visitor); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithDemoMode(DemoConfig{VisitorUserID: demoTestVisitorID, Username: "demo", Password: "demo-readonly"}))
	return &demoTestEnv{rt: rt, db: db, cookie: sessionCookieForTest(t, rt, demoTestVisitorID)}
}

func (e *demoTestEnv) do(method, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(""))
	req.AddCookie(e.cookie)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func demoErrCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body demoErrorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Error
}

func samplePath(p string) string {
	out := p
	for {
		i := strings.Index(out, "{")
		if i < 0 {
			return out
		}
		j := strings.Index(out[i:], "}")
		out = out[:i] + "x" + out[i+j+1:]
	}
}

// TestDemoGateEveryNonGETRoute walks the generated route table so a newly
// added route is blocked by default until it is deliberately allowlisted.
func TestDemoGateEveryNonGETRoute(t *testing.T) {
	t.Parallel()
	e := newDemoTestEnv(t)
	allowed := map[string]bool{}
	for _, a := range demoWriteAllowlist {
		allowed[a] = true
	}
	var nonGet []matrixRoute
	for _, rte := range openAPIRoutes {
		if rte.Method == http.MethodGet || rte.Method == http.MethodHead {
			continue
		}
		nonGet = append(nonGet, matrixRoute{method: rte.Method, pattern: rte.Path})
	}
	checked := len(nonGet)
	for _, msg := range probeParallel(nonGet, func(rte matrixRoute) string {
		key := rte.key()
		rec := e.do(rte.method, samplePath(rte.pattern), nil)
		code := demoErrCode(t, rec)
		if allowed[key] {
			if code == demoCodeReadonly {
				return key + " is allowlisted but was blocked"
			}
			return ""
		}
		if rec.Code != http.StatusForbidden || code != demoCodeReadonly {
			return fmt.Sprintf("%s: status = %d code = %q, want 403 %s", key, rec.Code, code, demoCodeReadonly)
		}
		return ""
	}) {
		t.Error(msg)
	}
	if checked < 100 {
		t.Fatalf("only %d non-GET routes enumerated, route table looks empty", checked)
	}
	for key := range allowed {
		found := false
		for _, rte := range openAPIRoutes {
			if rte.Method+" "+rte.Path == key {
				found = true
			}
		}
		if !found && !strings.Contains(key, "/demo/") {
			t.Errorf("allowlist entry %q matches no registered route", key)
		}
	}
}

func TestDemoGateAllowlistGolden(t *testing.T) {
	want := []string{
		"OPTIONS /api/v1/forms/{app}/{name}",
		"OPTIONS /api/v1/sentry/api/{project}/envelope/",
		"OPTIONS /api/v1/sentry/api/{project}/store/",
		"POST /api/v1/apps/{name}/validate-spec",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/logout",
		"POST /api/v1/demo/cta",
		"POST /api/v1/demo/session",
		"POST /api/v1/iam/policies/validate",
		"POST /api/v1/iam/policy-templates/{id}/render",
		"POST /api/v1/iam/preview",
		"POST /api/v1/pipelines/validate",
	}
	got := append([]string(nil), demoWriteAllowlist...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("allowlist changed, review each addition for writes and egress\n got: %v\nwant: %v", got, want)
	}
}

func TestDemoGateUnknownAndEdgeRequests(t *testing.T) {
	e := newDemoTestEnv(t)
	prefix := e.rt.authEngine.Prefix()
	tests := []struct {
		name     string
		method   string
		target   string
		mutate   func(*http.Request)
		wantCode string
	}{
		{"unknown path write", http.MethodPost, "/api/v1/not-a-route", nil, demoCodeReadonly},
		{"traversal into write route", http.MethodPost, "/api/v1/demo/../apps", nil, demoCodeReadonly},
		{"auth library post", http.MethodPost, prefix + "/login", nil, demoCodeDisabled},
		{"auth library get", http.MethodGet, prefix + "/session", nil, demoCodeDisabled},
		{"terminal websocket", http.MethodGet, "/api/v1/apps/web/terminal", func(r *http.Request) {
			r.Header.Set("Upgrade", "websocket")
		}, demoCodeDisabled},
		{"exec path", http.MethodGet, "/api/v1/apps/web/exec", nil, demoCodeDisabled},
		{"metrics", http.MethodGet, "/metrics", nil, demoCodeDisabled},
		{"bearer token", http.MethodGet, "/api/v1/apps", func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer abc")
		}, demoCodeTokens},
		{"put on read route", http.MethodPut, "/api/v1/apps/web", nil, demoCodeReadonly},
		{"delete", http.MethodDelete, "/api/v1/apps/web", nil, demoCodeReadonly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(tt.method, tt.target, tt.mutate)
			if rec.Code != http.StatusForbidden || demoErrCode(t, rec) != tt.wantCode {
				t.Errorf("status = %d code = %q, want 403 %q", rec.Code, demoErrCode(t, rec), tt.wantCode)
			}
			if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
				t.Errorf("X-Robots-Tag = %q, want noindex, nofollow", got)
			}
		})
	}
}

func TestDemoReadDenylistMatchesRegisteredRoutes(t *testing.T) {
	norm := func(p string) string {
		parts := strings.Split(p, "/")
		for i, s := range parts {
			if strings.HasPrefix(s, "{") {
				parts[i] = "{}"
			}
		}
		return strings.Join(parts, "/")
	}
	registered := map[string]bool{}
	for _, r := range openAPIRoutes {
		if r.Method == http.MethodGet {
			registered[norm(r.Path)] = true
		}
	}
	e := newDemoTestEnv(t)
	for _, pat := range demoReadDenylist {
		if !registered[norm(pat)] {
			t.Errorf("denylist entry %q matches no registered GET route, update the list", pat)
		}
		rec := e.do(http.MethodGet, samplePath(pat), nil)
		if rec.Code != http.StatusForbidden || demoErrCode(t, rec) != demoCodeDisabled {
			t.Errorf("GET %s: status = %d code = %q, want 403 %s", pat, rec.Code, demoErrCode(t, rec), demoCodeDisabled)
		}
	}
}

func TestDemoReadElevation(t *testing.T) {
	e := newDemoTestEnv(t)
	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus func(int) bool
	}{
		{"root only list is readable", http.MethodGet, "/api/v1/nodes", func(c int) bool { return c == http.StatusOK }},
		{"sensitive read stays closed", http.MethodGet, "/api/v1/databases/x/schema", func(c int) bool { return c == http.StatusForbidden }},
		{"write stays closed", http.MethodPost, "/api/v1/nodes/x/drain", func(c int) bool { return c == http.StatusForbidden }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(tt.method, tt.target, nil)
			if !tt.wantStatus(rec.Code) {
				t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}

	plain, db := newTestRouter(t)
	u := storeUserWithAbilitiesForTest(t, db, "viewer", []string{AbilityRead})
	c := sessionCookieForTest(t, plain, u.ID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	plain.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("non-demo read-only user on root route: status = %d, want 403", rec.Code)
	}
}

// demoSecretKeyRe matches JSON keys that normally carry credential material.
var demoSecretKeyRe = regexp.MustCompile(`(?i)secret|token|key|password`)

// demoUnmaskedSecrets walks decoded JSON and returns the key paths whose
// string value is non-empty and not obviously masked.
func demoUnmaskedSecrets(v any, at string) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && demoSecretKeyRe.MatchString(k) {
				if s != "" && !strings.Contains(s, "*") && !strings.Contains(s, "•") && !strings.EqualFold(s, "redacted") && !strings.EqualFold(s, "masked") {
					out = append(out, at+"."+k)
				}
				continue
			}
			out = append(out, demoUnmaskedSecrets(val, at+"."+k)...)
		}
	case []any:
		for i, val := range x {
			out = append(out, demoUnmaskedSecrets(val, at+"["+strconv.Itoa(i)+"]")...)
		}
	}
	return out
}

// TestDemoSecretBearingReadsStayMasked keeps credential-bearing reads either
// denied or free of unmasked secret-like values for the demo visitor.
func TestDemoSecretBearingReadsStayMasked(t *testing.T) {
	db := openTestDB(t)
	m, err := demo.Seed(t.Context(), db, demo.Options{Seed: 1, Anchor: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	if err != nil {
		t.Fatalf("demo.Seed() error = %v", err)
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithDemoMode(DemoConfig{VisitorUserID: demo.VisitorUserID, Username: demo.VisitorUsername, Password: demo.VisitorPassword}))
	e := &demoTestEnv{rt: rt, db: db, cookie: sessionCookieForTest(t, rt, demo.VisitorUserID)}
	routes := []string{
		"/api/v1/apps/{name}/secrets",
		"/api/v1/databases/{name}/key",
		"/api/v1/databases/{name}/keys",
		"/api/v1/mail/tokens",
		"/api/v1/nodes/join-tokens",
		"/api/v1/sso/scim/tokens",
		"/api/v1/backup-targets/{id}",
		"/api/v1/registry-credentials",
	}
	for _, pat := range routes {
		t.Run(pat, func(t *testing.T) {
			target := strings.NewReplacer("{name}", m.Services[0], "{id}", "1").Replace(pat)
			if strings.Contains(pat, "databases") {
				target = strings.Replace(pat, "{name}", m.Databases[0], 1)
			}
			rec := e.do(http.MethodGet, target, nil)
			if rec.Code == http.StatusForbidden {
				return
			}
			var body any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				return
			}
			if bad := demoUnmaskedSecrets(body, ""); len(bad) > 0 {
				t.Errorf("GET %s (status %d) returned unmasked values at %v", pat, rec.Code, bad)
			}
		})
	}
}
