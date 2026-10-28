package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

const cliConsoleSessionPath = "/api/v1/system/cli-console/session"

func cliConsoleStepUp(t *testing.T, rt *Router, cookie *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	body := `{"password":"` + testAdminPassword + `","action":"manage","app":"` + cliConsoleStepUpApp + `"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/step-up", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("step-up = %d %s", rec.Code, rec.Body.String())
	}
}

func cliConsoleRequest(t *testing.T, cookie *http.Cookie, path, origin string) *http.Request {
	t.Helper()
	var r *http.Request
	if cookie != nil {
		r = authedRequest(t, cookie, http.MethodGet, path, "")
	} else {
		r = httptest.NewRequest(http.MethodGet, path, nil)
	}
	r.Host = "example.com"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

// Every gate answers before anything is minted or run; the last case proves
// a fully authorised caller still stops at the missing binary, pre-upgrade.
func TestCLIConsole_GateOrder(t *testing.T) {
	t.Setenv(cliconsole.EnvBinary, "/nonexistent/cli")
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)

	serve := func(r *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, r)
		return rec
	}
	tests := []struct {
		name     string
		setup    func()
		cookie   *http.Cookie
		origin   string
		wantCode int
		wantErr  string
	}{
		{"unauthenticated", func() { experimental.Set(experimental.CLIConsole) }, nil, "http://example.com", http.StatusUnauthorized, ""},
		{"wrong ability", func() { experimental.Set(experimental.CLIConsole) }, readerCookie, "http://example.com", http.StatusForbidden, ""},
		{"flag off", func() { experimental.Set() }, admin, "http://example.com", http.StatusConflict, ExperimentalDisabledCode},
		{"kill switch wins", func() { experimental.Set(experimental.CLIConsole); t.Setenv(cliconsole.EnvKillSwitch, "off") }, admin, "http://example.com", http.StatusConflict, ExperimentalDisabledCode},
		{"missing origin", func() { experimental.Set(experimental.CLIConsole); t.Setenv(cliconsole.EnvKillSwitch, "") }, admin, "", http.StatusForbidden, errCodeCrossOrigin},
		{"cross origin", func() { experimental.Set(experimental.CLIConsole) }, admin, "http://evil.example", http.StatusForbidden, errCodeCrossOrigin},
		{"missing step-up", func() { experimental.Set(experimental.CLIConsole) }, admin, "http://example.com", http.StatusForbidden, errCodeStepUpRequired},
		{"binary missing after every gate", func() { cliConsoleStepUp(t, rt, admin) }, admin, "http://example.com", http.StatusConflict, errCodeCLIConsoleUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			rec := serve(cliConsoleRequest(t, tc.cookie, cliConsoleSessionPath, tc.origin))
			if rec.Code != tc.wantCode || (tc.wantErr != "" && errorCode(t, rec) != tc.wantErr) {
				t.Fatalf("status = %d code = %q body = %s, want %d %q", rec.Code, errorCode(t, rec), rec.Body.String(), tc.wantCode, tc.wantErr)
			}
		})
	}

	tokens, err := db.ListAPITokens(t.Context())
	if err != nil || len(tokens) != 0 {
		t.Fatalf("no session token may be minted before the upgrade, got %d (%v)", len(tokens), err)
	}
}

func TestCLIConsole_CommandsNeedStepUp(t *testing.T) {
	experimental.Set(experimental.CLIConsole)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	path := "/api/v1/system/cli-console/commands"

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, cliConsoleRequest(t, admin, path, ""))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != errCodeStepUpRequired {
		t.Fatalf("before step-up = %d %s", rec.Code, rec.Body.String())
	}
	cliConsoleStepUp(t, rt, admin)
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, cliConsoleRequest(t, admin, path, ""))
	var resp cliConsoleCommandsResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || len(resp.Commands) == 0 {
		t.Fatalf("after step-up = %d %s", rec.Code, rec.Body.String())
	}
	for _, c := range resp.Commands {
		if c.Name == "completion" || c.Name == "tui" {
			t.Fatalf("denied command %q offered for completion", c.Name)
		}
	}
}

func TestCLIConsole_OpenRateLimit(t *testing.T) {
	t.Setenv(cliconsole.EnvBinary, "/nonexistent/cli")
	experimental.Set(experimental.CLIConsole)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	rt.cliConsole.opens = newAPIRateLimiter(2)
	admin := loginTestSession(t, rt, db)
	cliConsoleStepUp(t, rt, admin)

	codes := []int{}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, cliConsoleRequest(t, admin, cliConsoleSessionPath, "http://example.com"))
		codes = append(codes, rec.Code)
	}
	if codes[0] != http.StatusConflict || codes[1] != http.StatusConflict || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("codes = %v, want two pre-upgrade refusals then 429", codes)
	}
}

func TestConsoleTokenAbilities(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
	}{
		{[]string{AbilityRoot}, []string{AbilityRead, AbilityWrite, AbilityDeploy}},
		{[]string{AbilityRead, AbilitySignInApprove, AbilitySecretsReveal}, []string{AbilityRead}},
		{[]string{AbilityReadSensitive, AbilityWriteSensitive, AbilityWrite}, []string{AbilityWrite}},
	}
	for _, tc := range tests {
		got := consoleTokenAbilities(tc.in)
		if !slices.Equal(got, tc.want) {
			t.Fatalf("consoleTokenAbilities(%v) = %v, want %v", tc.in, got, tc.want)
		}
		if hasAbility(got, AbilityRoot) || hasAbility(got, AbilityReadSensitive) || slices.Contains(got, AbilitySecretsReveal) {
			t.Fatalf("console token %v carries root, sensitive or reveal access", got)
		}
	}
}

func TestCLIConsoleAPIURL(t *testing.T) {
	tests := map[string]string{":8080": "http://127.0.0.1:8080", "0.0.0.0:9": "http://127.0.0.1:9", "[::]:9": "http://127.0.0.1:9", "10.0.0.2:80": "http://10.0.0.2:80", "": ""}
	for in, want := range tests {
		rt := &Router{dashboardListenAddr: in}
		if got := rt.cliConsoleAPIURL(); got != want {
			t.Fatalf("cliConsoleAPIURL(%q) = %q, want %q", in, got, want)
		}
	}
}
