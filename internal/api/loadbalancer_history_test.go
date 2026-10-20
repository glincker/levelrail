package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
)

type failProber struct{}

func (failProber) Probe(context.Context, string, string, *loadbalancer.UpstreamTLS, time.Duration) loadbalancer.ProbeResult {
	return loadbalancer.ProbeResult{StatusCode: 503, Latency: 3 * time.Millisecond, CheckedAt: time.Now(), Err: "status 503"}
}

func seedLBApp(t *testing.T, cfg string) (*Router, *store.DB, *http.Cookie, *loadbalancer.Registry) {
	t.Helper()
	reg := loadbalancer.NewRegistry()
	rt, db, cookie := newLBRouter(t, reg, fakeLBStats{"127.0.0.1:1": {NumRequests: 3}})
	seedWebAppForTest(t, db)
	if rec := lbDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/loadbalancer", cfg); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	reg.Record(loadbalancer.Observation{
		Service: "web", Ready: true, Reason: "Balancing", ObservedAt: time.Now(),
		Upstreams: []loadbalancer.UpstreamObservation{
			{Upstream: loadbalancer.Upstream{ID: "web#0", Dial: "127.0.0.1:1", Replica: 0}, Running: true},
			{Upstream: loadbalancer.Upstream{ID: "web#1", Dial: "127.0.0.1:2", Replica: 1}, Running: true},
		},
	})
	return rt, db, cookie, reg
}

func TestLoadBalancerHistory_RecordsStatusReadsAndTransitions(t *testing.T) {
	rt, _, cookie, _ := seedLBApp(t, `{"active_health":{"path":"/h","timeout":"2s"}}`)

	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/status", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	rt.lb.prober = failProber{}
	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/status", ""); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/history?limit=1", "")
	var res lbHistoryResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || len(res.Upstreams) != 2 {
		t.Fatalf("history = %d %s", rec.Code, rec.Body.String())
	}
	u := res.Upstreams[0]
	if u.AdminState != loadbalancer.AdminActive || len(u.Checks) != 1 || u.Checks[0].OK || u.Checks[0].Reason != "expected 2xx or 3xx, got 503" {
		t.Errorf("upstream 0 = %+v", u)
	}
	if len(u.Transitions) != 1 || u.Transitions[0].From != loadbalancer.StateHealthy || u.Transitions[0].To != loadbalancer.StateUnhealthy {
		t.Errorf("transitions = %+v", u.Transitions)
	}
	if len(u.Series.Connections) == 0 || u.Series.Connections[0].Value != 3 {
		t.Errorf("series = %+v", u.Series)
	}

	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/history?limit=abc", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("bad limit = %d, want 400", rec.Code)
	}
}

func TestLoadBalancerHistory_EmptyBeforeFirstPassAndUnconfigured(t *testing.T) {
	reg := loadbalancer.NewRegistry()
	rt, db, cookie := newLBRouter(t, reg, nil)
	seedWebAppForTest(t, db)
	if rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/history", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unconfigured = %d, want 404", rec.Code)
	}
	lbDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/loadbalancer", `{}`)
	rec := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/history", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"upstreams":[]`) {
		t.Fatalf("before first pass = %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoadBalancerCheck_RunsProbesAndRateLimits(t *testing.T) {
	rt, _, cookie, reg := seedLBApp(t, `{}`)
	rt.lb.gate = loadbalancer.NewCheckGate(time.Hour)

	rec := lbDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/loadbalancer/check", "")
	var res lbCheckResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || len(res.Results) != 2 {
		t.Fatalf("check = %d %s", rec.Code, rec.Body.String())
	}
	if !res.Results[0].OK || res.Results[0].StatusCode != 200 || res.Note == "" {
		t.Errorf("results = %+v note %q, want ok and a note about the default probe", res.Results, res.Note)
	}
	obs, _ := reg.Get("web")
	hist := reg.History(loadbalancer.BuildStatus(context.Background(), obs, nil, nil), 0)
	if len(hist[0].Checks) != 1 {
		t.Errorf("check-now must record into history, got %d", len(hist[0].Checks))
	}

	rec = lbDo(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/loadbalancer/check", "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("second check = %d retry-after %q, want 429", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestLoadBalancerUpstreamAdminState(t *testing.T) {
	rt, db, cookie, _ := seedLBApp(t, `{}`)
	put := func(id, body string) *httptest.ResponseRecorder {
		return lbDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/loadbalancer/upstreams/"+id, body)
	}

	rec := put("web%230", `{"admin_state":"draining"}`)
	var st loadbalancer.UpstreamStatus
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &st) != nil || st.AdminState != loadbalancer.AdminDraining || st.State != loadbalancer.StateDraining {
		t.Fatalf("PUT draining = %d %s", rec.Code, rec.Body.String())
	}
	states, err := db.ListLBUpstreamAdminStates(context.Background())
	if err != nil || states["web"][0] != loadbalancer.AdminDraining {
		t.Fatalf("persisted = %v err %v", states, err)
	}

	status := lbDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/loadbalancer/status", "")
	if !strings.Contains(status.Body.String(), `"admin_state":"draining"`) || !strings.Contains(status.Body.String(), `"last_changed_at"`) {
		t.Errorf("status must expose admin_state and last_changed_at: %s", status.Body.String())
	}

	if rec := put("web%230", `{"admin_state":"active"}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT active = %d", rec.Code)
	}
	if states, _ := db.ListLBUpstreamAdminStates(context.Background()); len(states) != 0 {
		t.Errorf("active must clear the row, got %v", states)
	}

	tests := []struct {
		name, id, body string
		want           int
	}{
		{"bad state", "web%230", `{"admin_state":"paused"}`, http.StatusBadRequest},
		{"unknown field", "web%230", `{"admin_state":"active","x":1}`, http.StatusBadRequest},
		{"unknown upstream", "web%239", `{"admin_state":"disabled"}`, http.StatusNotFound},
		{"foreign id", "other%230", `{"admin_state":"disabled"}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := put(tt.id, tt.body); rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestLoadBalancerHistoryRoutes_IAMDenyAndAbilities(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithLoadBalancers(db, loadbalancer.NewRegistry(), nil))
	bootstrapTestAdmin(t, db)
	for _, name := range []string{"secret-app", "open-app"} {
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: name, Image: "img:v1", Port: 80}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetServiceLoadBalancer(context.Background(), name, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	abilities := []string{AbilityRead, AbilityWrite}
	denied := storeUserWithAbilitiesForTest(t, db, "denied@example.com", abilities)
	readOnly := storeUserWithAbilitiesForTest(t, db, "ro@example.com", []string{AbilityRead})
	for _, ability := range abilities {
		attachTestPolicy(t, db, "deny-secret-"+ability, "Deny", ability, "app:secret-app", store.PrincipalTypeUser, denied.ID)
	}
	deniedCookie := sessionCookieForTest(t, rt, denied.ID)
	roCookie := sessionCookieForTest(t, rt, readOnly.ID)

	do := func(cookie *http.Cookie, method, target, body string) int {
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, authedRequest(t, cookie, method, target, body))
		return w.Code
	}
	routes := []struct{ method, path, body string }{
		{http.MethodGet, "history", ""},
		{http.MethodPost, "check", ""},
		{http.MethodPut, "upstreams/secret-app%230", `{"admin_state":"disabled"}`},
	}
	for _, rc := range routes {
		t.Run(rc.method+" "+rc.path, func(t *testing.T) {
			if got := do(deniedCookie, rc.method, "/api/v1/apps/secret-app/loadbalancer/"+rc.path, rc.body); got != http.StatusForbidden {
				t.Errorf("denied user on secret-app = %d, want 403", got)
			}
			if got := do(deniedCookie, rc.method, "/api/v1/apps/open-app/loadbalancer/"+strings.ReplaceAll(rc.path, "secret-app", "open-app"), rc.body); got == http.StatusForbidden {
				t.Errorf("denied user on open-app = %d, want the handler's status", got)
			}
			if rc.method != http.MethodGet {
				if got := do(roCookie, rc.method, "/api/v1/apps/open-app/loadbalancer/"+strings.ReplaceAll(rc.path, "secret-app", "open-app"), rc.body); got != http.StatusForbidden {
					t.Errorf("read-only user mutating = %d, want 403", got)
				}
			}
		})
	}
}
