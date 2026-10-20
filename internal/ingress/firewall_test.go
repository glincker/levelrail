package ingress

import (
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/edgefw"
)

func firewallFixture(t *testing.T, rules []edgefw.Rule, s edgefw.Settings) *firewallModule {
	t.Helper()
	eng := edgefw.Default()
	eng.SetRules(rules)
	eng.SetSettings(s)
	t.Cleanup(func() {
		eng.SetRules(nil)
		eng.SetSettings(edgefw.DefaultSettings())
		for _, b := range eng.Bans.List(time.Now()) {
			eng.Bans.Lift(b.IP)
		}
	})
	m := &firewallModule{}
	if err := m.Provision(caddy.Context{}); err != nil {
		t.Fatal(err)
	}
	return m
}

func fwDo(m *firewallModule, method, target, remote, ua string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = remote
	req.Header.Set("User-Agent", ua)
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	ok := caddyhttp.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
	_ = m.ServeHTTP(rec, req, ok)
	return rec
}

func fwRule(id string, pos int, a edgefw.Action, c ...edgefw.Condition) edgefw.Rule {
	return edgefw.Rule{ID: id, Name: id, Enabled: true, Position: pos, Action: a, Conditions: c}
}

func TestFirewallModule_RulesAndBans(t *testing.T) {
	m := firewallFixture(t, []edgefw.Rule{
		fwRule("allow", 1, edgefw.ActionAllow, edgefw.Condition{Field: edgefw.FieldIP, Op: edgefw.OpInCIDR, Value: "198.51.100.200"}),
		fwRule("deny", 2, edgefw.ActionDeny, edgefw.Condition{Field: edgefw.FieldPath, Op: edgefw.OpStartsWith, Value: "/admin"}),
	}, edgefw.DefaultSettings())
	tests := []struct {
		name, path, remote string
		want               int
	}{
		{"unmatched passes", "/home", "198.51.100.1:1", http.StatusNoContent},
		{"deny rule", "/admin/x", "198.51.100.1:1", http.StatusForbidden},
		{"allow wins", "/admin/x", "198.51.100.200:1", http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fwDo(m, http.MethodGet, tt.path, tt.remote, "Mozilla/5.0", nil, "").Code; got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
	eng := edgefw.Default()
	lim := edgefw.DefaultLimits()
	lim.BanStrikes = 3
	eng.Bans.SetLimits(lim)
	t.Cleanup(func() { eng.Bans.SetLimits(edgefw.DefaultLimits()) })
	for i := 0; i < 3; i++ {
		fwDo(m, http.MethodGet, "/admin", "203.0.113.50:1", "x", nil, "")
	}
	if got := fwDo(m, http.MethodGet, "/home", "203.0.113.50:1", "x", nil, "").Code; got != http.StatusForbidden {
		t.Fatalf("banned client got %d", got)
	}
	if len(eng.Bans.List(time.Now())) != 1 {
		t.Fatal("expected one ban")
	}
}

func TestFirewallModule_RateLimitRule(t *testing.T) {
	rl := fwRule("rl", 1, edgefw.ActionRateLimit, edgefw.Condition{Field: edgefw.FieldPath, Op: edgefw.OpStartsWith, Value: "/api"})
	rl.RateCount, rl.RateWindowSeconds = 2, 60
	m := firewallFixture(t, []edgefw.Rule{rl}, edgefw.DefaultSettings())
	var codes []int
	for i := 0; i < 3; i++ {
		rec := fwDo(m, http.MethodGet, "/api/x", "198.51.100.77:1", "x", nil, "")
		codes = append(codes, rec.Code)
		if rec.Code == 429 && rec.Header().Get("Retry-After") == "" {
			t.Fatal("429 without Retry-After")
		}
	}
	if codes[0] != 204 || codes[1] != 204 || codes[2] != 429 {
		t.Fatalf("codes = %v", codes)
	}
}

func TestFirewallModule_ChallengeFlow(t *testing.T) {
	s := edgefw.DefaultSettings()
	s.AIBotMode = edgefw.ModeChallenge
	m := firewallFixture(t, nil, s)
	rec := fwDo(m, http.MethodGet, "/", "198.51.100.5:1", "Mozilla GPTBot", nil, "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("expected challenge page, got %d", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("csp = %q", csp)
	}
	page := rec.Body.String()
	i := strings.Index(page, `id="lr-token" value="`)
	token := page[i+len(`id="lr-token" value="`):]
	token = token[:strings.Index(token, `"`)]
	counter := ""
	bits := edgefw.Default().Limits().ChallengeBits
	for n := 0; ; n++ {
		if edgefw.LeadingZeroBits(sha256.Sum256([]byte(token+":"+strconv.Itoa(n)))) >= bits {
			counter = strconv.Itoa(n)
			break
		}
	}
	bad := fwDo(m, http.MethodPost, edgefw.ChallengePath, "198.51.100.5:1", "x", nil, url.Values{"t": {token}, "n": {"zzz"}}.Encode())
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad proof = %d", bad.Code)
	}
	elsewhere := fwDo(m, http.MethodPost, edgefw.ChallengePath, "203.0.113.5:1", "x", nil, url.Values{"t": {token}, "n": {counter}}.Encode())
	if elsewhere.Code != http.StatusBadRequest {
		t.Fatalf("token from another network accepted: %d", elsewhere.Code)
	}
	ok := fwDo(m, http.MethodPost, edgefw.ChallengePath, "198.51.100.5:1", "x", nil, url.Values{"t": {token}, "n": {counter}}.Encode())
	if ok.Code != http.StatusNoContent {
		t.Fatalf("good proof = %d", ok.Code)
	}
	cookies := ok.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Name != edgefw.PassCookie {
		t.Fatalf("cookies = %+v", cookies)
	}
	if got := fwDo(m, http.MethodGet, "/", "198.51.100.5:1", "Mozilla GPTBot", cookies[0], "").Code; got != http.StatusNoContent {
		t.Fatalf("pass cookie not honored: %d", got)
	}
}

func TestFirewallModule_ShieldOutcomeClassifiedOnce(t *testing.T) {
	m := firewallFixture(t, nil, edgefw.DefaultSettings())
	eng := edgefw.Default()
	before := eng.Stats.Totals("", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.90:1"
	rec := httptest.NewRecorder()
	handler := caddyhttp.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) error {
		w.WriteHeader(http.StatusTooManyRequests)
		return nil
	})
	_ = m.ServeHTTP(rec, req, handler)
	after := eng.Stats.Totals("", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	var diff uint64
	for i := range after {
		diff += after[i] - before[i]
	}
	if diff != 1 {
		t.Fatalf("one request recorded %d times", diff)
	}
}

func TestCookieValue(t *testing.T) {
	tests := []struct{ header, want string }{
		{"a=1; __lr_pass=tok.sig; b=2", "tok.sig"},
		{"__lr_pass=x", "x"},
		{"a=1", ""},
		{"", ""},
		{"x__lr_pass=1", ""},
	}
	for _, tt := range tests {
		if got := cookieValue(tt.header, edgefw.PassCookie); got != tt.want {
			t.Errorf("cookieValue(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestBuildRoutesConfig_FirewallRouteFirst(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "s", ListenAddr: ":443", Firewall: true,
		Routes: []ProxyRoute{{Hosts: []string{"a.example.com"}, BackendDial: "127.0.0.1:9000"}}})
	if err != nil {
		t.Fatal(err)
	}
	routes := cfg.Apps.HTTP.Servers["s"].Routes
	if len(routes) != 2 || len(routes[0].Match) != 0 {
		t.Fatalf("routes = %+v", routes)
	}
	if h, ok := routes[0].Handle[0].(FirewallHandler); !ok || h.Handler != "levelrail_firewall" {
		t.Fatalf("first handler = %+v", routes[0].Handle[0])
	}
}
