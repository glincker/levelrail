package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

type goldenStep struct {
	Label  string
	Status int
	Body   any
}

// goldenRouter builds a router in the given engine mode over a fresh database.
func goldenRouter(t *testing.T, mode string) (*Router, *http.Cookie, *store.DB) {
	t.Helper()
	return goldenRouterPoll(t, mode, "1ms")
}

func goldenRouterPoll(t *testing.T, mode, pollInterval string) (*Router, *http.Cookie, *store.DB) {
	t.Helper()
	t.Setenv(authengine.EnvEngine, mode)
	t.Setenv(authengine.EnvAreas, "")
	t.Setenv(authengine.EnvPollInterval, pollInterval)
	db := openTestDB(t)
	var opts []Option
	if mode != authengine.EngineLegacy {
		eng, err := authengine.New(db.DB, authengine.Config{
			BaseURL: "http://golden.test", TokenPrefix: testBrand().ShortName, Directory: authengine.NewDirectory(db.DB),
			DeviceTokenTTL: DeviceTokenTTL(), DeviceCodeTTL: DeviceCodeTTL(),
		})
		if err != nil {
			t.Fatalf("authengine.New: %v", err)
		}
		t.Cleanup(eng.Close)
		if mode == authengine.EngineShadow {
			opts = append(opts, WithAuthEngineShadow(eng, authengine.ShadowConfig{QueueSize: 8, Workers: 1, MismatchLog: 5}))
		} else {
			opts = append(opts, WithAuthEngineLibrary(eng))
		}
	}
	rt := NewRouter(discardLogger(), testBrand(), db, opts...)
	t.Cleanup(rt.CloseAuthShadow)
	return rt, loginTestSession(t, rt, db), db
}

var goldenVolatile = map[string]bool{
	"id": true, "created_at": true, "expires_at": true, "last_used_at": true, "revoked_at": true,
	"token": true, "device_code": true, "user_code": true, "verification_uri_complete": true,
}

// goldenNormalize keeps a body's shape and drops the values that differ per run.
func goldenNormalize(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = goldenNormalize(val, k)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = goldenNormalize(val, key)
		}
		return out
	case string:
		if goldenVolatile[key] {
			return "<" + key + ">"
		}
	}
	return v
}

type goldenRun struct {
	t      *testing.T
	rt     *Router
	cookie *http.Cookie
	steps  []goldenStep
}

func (g *goldenRun) do(label string, req *http.Request) map[string]any {
	g.t.Helper()
	rec := httptest.NewRecorder()
	g.rt.Handler().ServeHTTP(rec, req)
	var body any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			g.t.Fatalf("%s: decode %q: %v", label, rec.Body.String(), err)
		}
	}
	g.steps = append(g.steps, goldenStep{Label: label, Status: rec.Code, Body: goldenNormalize(body, "")})
	m, _ := body.(map[string]any)
	return m
}

func (g *goldenRun) session(method, target, body string) *http.Request {
	return authedRequest(g.t, g.cookie, method, target, body)
}

func goldenBearer(method, token, body string) *http.Request {
	r := httptest.NewRequest(method, "/api/v1/apps", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func goldenScenario(t *testing.T, mode string) []goldenStep {
	rt, cookie, _ := goldenRouter(t, mode)
	g := &goldenRun{t: t, rt: rt, cookie: cookie}

	g.do("create rejects empty name", g.session(http.MethodPost, "/api/v1/auth/tokens", `{"name":"","abilities":["read"]}`))
	g.do("create rejects unknown ability", g.session(http.MethodPost, "/api/v1/auth/tokens", `{"name":"x","abilities":["nope"]}`))
	read := g.do("create read token", g.session(http.MethodPost, "/api/v1/auth/tokens", `{"name":"reader","abilities":["read"]}`))
	g.do("create agent token", g.session(http.MethodPost, "/api/v1/auth/tokens", `{"name":"bot","abilities":["read","deploy"],"expires_in_days":7,"agent":{"name":"claude","description":"ci"}}`))
	g.do("list", g.session(http.MethodGet, "/api/v1/auth/tokens", ""))

	readToken := fmt.Sprint(read["token"])
	g.do("read token reads", goldenBearer(http.MethodGet, readToken, ""))
	g.do("read token cannot write", goldenBearer(http.MethodPost, readToken, `{}`))
	g.do("garbage token", goldenBearer(http.MethodGet, "not-a-token", ""))
	g.do("revoke", g.session(http.MethodDelete, "/api/v1/auth/tokens/"+fmt.Sprint(read["id"]), ""))
	g.do("revoked token", goldenBearer(http.MethodGet, readToken, ""))
	g.do("revoke unknown", g.session(http.MethodDelete, "/api/v1/auth/tokens/tok_missing", ""))
	g.do("list after revoke", g.session(http.MethodGet, "/api/v1/auth/tokens", ""))

	start := g.do("device start", httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", strings.NewReader(`{"client_name":"laptop"}`)))
	code, userCode := fmt.Sprint(start["device_code"]), fmt.Sprint(start["user_code"])
	poll := func(label string) map[string]any {
		return g.do(label, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"`+code+`"}`)))
	}
	poll("device poll pending")
	g.do("device pending list", g.session(http.MethodGet, "/api/v1/auth/device/requests", ""))
	g.do("device approve unknown", g.session(http.MethodPost, "/api/v1/auth/device/ZZZZ-ZZZZ/approve", ""))
	g.do("device approve", g.session(http.MethodPost, "/api/v1/auth/device/"+userCode+"/approve", ""))
	granted := poll("device poll granted")
	poll("device poll redeemed")
	g.do("device token works", goldenBearer(http.MethodGet, fmt.Sprint(granted["token"]), ""))

	deny := g.do("device start 2", httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", strings.NewReader(`{}`)))
	g.do("device deny", g.session(http.MethodPost, "/api/v1/auth/device/"+fmt.Sprint(deny["user_code"])+"/deny", ""))
	g.do("device poll denied", httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"`+fmt.Sprint(deny["device_code"])+`"}`)))
	g.do("device poll unknown code", httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"nope"}`)))
	return g.steps
}

// Device token names differ by design ("cli login" vs "device login") and the test shortens the poll interval; only those fields are masked.
func maskDeviceName(steps []goldenStep) {
	for _, s := range steps {
		m, ok := s.Body.(map[string]any)
		switch {
		case !ok:
		case s.Label == "device poll granted":
			m["name"] = "<name>"
		case strings.HasPrefix(s.Label, "device start"):
			m["interval"] = "<interval>"
		}
	}
}

func TestAuthEngineGoldenContract(t *testing.T) {
	legacy := goldenScenario(t, authengine.EngineLegacy)
	maskDeviceName(legacy)
	for _, mode := range []string{authengine.EngineShadow, authengine.EngineLibrary} {
		t.Run(mode, func(t *testing.T) {
			got := goldenScenario(t, mode)
			maskDeviceName(got)
			if len(got) != len(legacy) {
				t.Fatalf("steps = %d, want %d", len(got), len(legacy))
			}
			for i := range legacy {
				if got[i].Status != legacy[i].Status || !reflect.DeepEqual(got[i].Body, legacy[i].Body) {
					t.Errorf("%s: %s differs from legacy\n got  %d %v\n want %d %v", mode, legacy[i].Label, got[i].Status, got[i].Body, legacy[i].Status, legacy[i].Body)
				}
			}
		})
	}
}

// Within the poll interval the library answers slow-down, which must not mask a redeemed code.
func TestAuthEngineRedeemedDeviceCodeIsExpiredWithinInterval(t *testing.T) {
	type outcome struct {
		status int
		body   string
	}
	run := func(mode string) []outcome {
		rt, cookie, _ := goldenRouterPoll(t, mode, "1h")
		g := &goldenRun{t: t, rt: rt, cookie: cookie}
		start := g.do("start", httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", strings.NewReader(`{}`)))
		code := fmt.Sprint(start["device_code"])
		g.do("approve", g.session(http.MethodPost, "/api/v1/auth/device/"+fmt.Sprint(start["user_code"])+"/approve", ""))
		var out []outcome
		for range 3 {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"`+code+`"}`)))
			out = append(out, outcome{rec.Code, strings.TrimSpace(rec.Body.String())})
		}
		return out
	}
	legacy := run(authengine.EngineLegacy)
	if legacy[0].status != http.StatusOK || legacy[1].status != http.StatusBadRequest || !strings.Contains(legacy[1].body, "expired_token") {
		t.Fatalf("legacy baseline unexpected: %+v", legacy)
	}
	got := run(authengine.EngineLibrary)
	if got[0].status != legacy[0].status || got[1] != legacy[1] || got[2] != legacy[2] {
		t.Errorf("library = %+v, want legacy shape %+v", got[1:], legacy[1:])
	}
}
