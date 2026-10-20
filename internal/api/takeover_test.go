package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/takeover"
)

type takeoverFakeDocker struct {
	docker.Runtime
	mu         sync.Mutex
	containers []docker.LocalContainer
	stopped    []string
	started    []string
	stopErr    error
}

func (f *takeoverFakeDocker) ListLocalContainers(context.Context) ([]docker.LocalContainer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]docker.LocalContainer(nil), f.containers...), nil
}

func (f *takeoverFakeDocker) InspectLocalContainer(_ context.Context, name string) (docker.LocalContainer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.containers {
		if c.Name == name {
			c.RestartPolicy = "unless-stopped"
			return c, nil
		}
	}
	return docker.LocalContainer{}, errors.New("not found")
}

func (f *takeoverFakeDocker) BridgeGatewayIP(context.Context) (string, error) { return "", nil }

func (f *takeoverFakeDocker) Stop(_ context.Context, name string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopErr != nil {
		return f.stopErr
	}
	f.stopped = append(f.stopped, name)
	return nil
}

func (f *takeoverFakeDocker) Start(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, name)
	return nil
}

type takeoverEnv struct {
	rt      *Router
	db      *store.DB
	cookie  *http.Cookie
	docker  *takeoverFakeDocker
	fired   atomic.Int32
	probeOK atomic.Bool
}

func newTakeoverEnv(t *testing.T) *takeoverEnv {
	t.Helper()
	e := &takeoverEnv{docker: &takeoverFakeDocker{containers: []docker.LocalContainer{
		{Name: "coolify-proxy", Image: "traefik:v3.1", Running: true, Published: []int{80, 443}},
	}}}
	db := openTestDB(t)
	e.db = db
	e.rt = NewRouter(discardLogger(), testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return e.docker, nil }), WithDoctorIngressPorts(8088, 8443))
	e.rt.publicHost = "203.0.113.9"
	e.rt.lookupHost = func(context.Context, string) ([]string, error) { return []string{"203.0.113.9"}, nil }
	e.rt.takeover = takeoverService{
		hostSS:        func(context.Context) (string, error) { return "", errors.New("no ss") },
		portListening: func(int) bool { return false },
		diskFreeGB:    func() (float64, bool) { return 50, true },
		memoryMB:      func() int { return 4096 },
		cfg:           &takeover.Config{Window: 30 * time.Millisecond, VerifyInterval: 5 * time.Millisecond, StartWait: time.Millisecond},
		probe: func(_ context.Context, _ proxyroutes.ProbeTarget) proxyroutes.ProbeResult {
			ok := e.probeOK.Load()
			return proxyroutes.ProbeResult{Reachable: ok, CertTrusted: ok, StatusCode: 200, Delivered: ok, Err: "refused"}
		},
	}
	e.rt.SetRestarter(Restarter{Supervised: true, Trigger: func() { e.fired.Add(1) }})
	e.cookie = loginTestSession(t, e.rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "shop", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.EditServiceDomains(ctx, "shop", func([]string) ([]string, error) { return []string{"shop.example.com"}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertConditions(ctx, applicationControllerName("shop"), []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateIngressSettings(ctx, store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com", TLSTerminatedUpstream: true, PublicHTTPSPort: 8443}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *takeoverEnv) do(t *testing.T, method, path, body string) (*httptest.ResponseRecorder, map[string]json.RawMessage) {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.cookie, method, path, body))
	var out map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func decodeRaw[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return v
}

func TestTakeoverPlanReportsHoldersDomainsAndChecks(t *testing.T) {
	e := newTakeoverEnv(t)
	rec, out := e.do(t, http.MethodGet, "/api/v1/migration/takeover/plan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	plan := decodeRaw[takeover.Plan](t, out["plan"])
	if plan.Verdict != takeover.VerdictWarn {
		t.Fatalf("verdict = %s: %+v", plan.Verdict, plan.Checks)
	}
	if len(plan.ProxyNames) != 1 || plan.ProxyNames[0] != "coolify-proxy" || len(plan.Domains) != 1 || plan.Domains[0].App != "shop" {
		t.Fatalf("plan = %+v", plan)
	}
	if len(e.docker.stopped) != 0 || e.fired.Load() != 0 {
		t.Fatal("a plan must change nothing")
	}
}

func TestTakeoverPlanFlagsHostListenerAndDuplicateDomain(t *testing.T) {
	e := newTakeoverEnv(t)
	e.docker.containers = []docker.LocalContainer{
		{Name: "old-shop", Image: "shop:1", Running: true, Labels: map[string]string{"traefik.http.routers.shop.rule": "Host(`shop.example.com`)"}},
	}
	e.rt.takeover.portListening = func(port int) bool { return port == 80 }
	_, out := e.do(t, http.MethodGet, "/api/v1/migration/takeover/plan", "")
	plan := decodeRaw[takeover.Plan](t, out["plan"])
	got := map[string]string{}
	for _, c := range plan.Checks {
		got[c.ID] = c.Status
	}
	if plan.Verdict != takeover.VerdictBlocked || got[takeover.CheckPorts] != takeover.StatusBlock || got[takeover.CheckConflict] != takeover.StatusWarn {
		t.Fatalf("verdict=%s checks=%v", plan.Verdict, got)
	}
}

func TestTakeoverDryRunChangesNothingAndRunNeedsConfirm(t *testing.T) {
	e := newTakeoverEnv(t)
	rec, out := e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs", `{}`)
	if rec.Code != http.StatusOK || string(out["dry_run"]) != "true" {
		t.Fatalf("dry run = %d %s", rec.Code, rec.Body)
	}
	if len(e.docker.stopped) != 0 || e.fired.Load() != 0 {
		t.Fatal("dry run touched the system")
	}
	if runs, _ := e.db.ListPortTakeovers(context.Background(), 5); len(runs) != 0 {
		t.Fatal("dry run recorded a run")
	}
}

func TestTakeoverRunBlockedPlanIs409AndTouchesNothing(t *testing.T) {
	e := newTakeoverEnv(t)
	e.docker.containers = nil
	e.rt.takeover.portListening = func(port int) bool { return port == 443 }
	rec, _ := e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs", `{"confirm":true}`)
	if rec.Code != http.StatusConflict || len(e.docker.stopped) != 0 || e.fired.Load() != 0 {
		t.Fatalf("status %d stopped=%v fired=%d", rec.Code, e.docker.stopped, e.fired.Load())
	}
}

func TestTakeoverRunIsIdempotentAndRollsBackThroughTheAPI(t *testing.T) {
	e := newTakeoverEnv(t)
	rec, out := e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs", `{"confirm":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	run := decodeRaw[takeoverRunResource](t, out["run"])
	if run.State != takeover.StateApplying || len(e.docker.stopped) != 1 || e.docker.stopped[0] != "coolify-proxy" {
		t.Fatalf("run = %+v stopped=%v", run, e.docker.stopped)
	}
	settings, _ := e.db.GetIngressSettings(context.Background())
	if settings.TLSTerminatedUpstream || settings.PublicHTTPSPort != 0 {
		t.Fatalf("ingress settings = %+v", settings)
	}
	rec, _ = e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs", `{"confirm":true}`)
	if rec.Code != http.StatusOK || len(e.docker.stopped) != 1 {
		t.Fatalf("repeat = %d stopped=%v", rec.Code, e.docker.stopped)
	}
	waitFor(t, 2*time.Second, func() bool { return e.fired.Load() == 1 })

	rec, out = e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs/"+run.ID+"/rollback", `{}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body)
	}
	if got := decodeRaw[takeoverRunResource](t, out["run"]); got.State != takeover.StateRollingBack {
		t.Fatalf("state = %s", got.State)
	}
	if _, err := takeover.PrepareBoot(context.Background(), takeover.DBStore{DB: e.db}, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.rt.ResumeTakeover(context.Background())
	if rec, _ := e.do(t, http.MethodGet, "/api/v1/migration/takeover/runs/"+run.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("get run = %d", rec.Code)
	}
	got, err := e.db.GetPortTakeover(context.Background(), run.ID)
	if err != nil || got.State != takeover.StateRolledBack {
		t.Fatalf("stored state = %s err=%v", got.State, err)
	}
	if len(e.docker.started) != 1 || e.docker.started[0] != "coolify-proxy" {
		t.Fatalf("started = %v", e.docker.started)
	}
	settings, _ = e.db.GetIngressSettings(context.Background())
	if !settings.TLSTerminatedUpstream || settings.PublicHTTPSPort != 8443 {
		t.Fatalf("settings not restored: %+v", settings)
	}
}

func TestTakeoverBindFailureRollsBackAndStartsOldProxy(t *testing.T) {
	e := newTakeoverEnv(t)
	_, out := e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs", `{"confirm":true}`)
	run := decodeRaw[takeoverRunResource](t, out["run"])
	st := takeover.DBStore{DB: e.db}
	for i := 0; i < 2; i++ {
		if _, err := takeover.PrepareBoot(context.Background(), st, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	e.rt.ResumeTakeover(context.Background())
	got, _ := e.db.GetPortTakeover(context.Background(), run.ID)
	if got.State != takeover.StateRolledBack || len(e.docker.started) != 1 {
		t.Fatalf("state=%s started=%v err=%q", got.State, e.docker.started, got.Error)
	}
}

func TestTakeoverStopFailureReportsAndLeavesSettings(t *testing.T) {
	e := newTakeoverEnv(t)
	e.docker.stopErr = errors.New("permission denied")
	rec, out := e.do(t, http.MethodPost, "/api/v1/migration/takeover/runs", `{"confirm":true}`)
	if rec.Code != http.StatusConflict || !strings.Contains(string(out["error"]), "permission denied") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	settings, _ := e.db.GetIngressSettings(context.Background())
	if !settings.TLSTerminatedUpstream || e.fired.Load() != 0 {
		t.Fatalf("settings=%+v fired=%d", settings, e.fired.Load())
	}
}

func TestTakeoverVerifierNeedsTheIngressToAnswer(t *testing.T) {
	e := newTakeoverEnv(t)
	v := takeoverVerifier{rt: e.rt}
	if got := v.Verify(context.Background(), "shop.example.com"); got.OK {
		t.Fatalf("an unreachable domain verified: %+v", got)
	}
	e.probeOK.Store(true)
	if got := v.Verify(context.Background(), "shop.example.com"); !got.OK || !got.Delivered {
		t.Fatalf("verify = %+v", got)
	}
}

func TestTakeoverRoutesAreRootOnly(t *testing.T) {
	e := newTakeoverEnv(t)
	routes := []routeCase{
		{http.MethodGet, "/api/v1/migration/takeover/plan"},
		{http.MethodGet, "/api/v1/migration/takeover/runs"},
		{http.MethodPost, "/api/v1/migration/takeover/runs"},
		{http.MethodGet, "/api/v1/migration/takeover/runs/x"},
		{http.MethodPost, "/api/v1/migration/takeover/runs/x/rollback"},
	}
	assertRoutesRequireAuth(t, e.rt, routes)
	var pcs []providerRouteCase
	for _, r := range routes {
		pcs = append(pcs, providerRouteCase{method: r.method, path: r.target, body: "{}"})
	}
	assertProviderRoutesForbiddenForAbilities(t, e.rt, e.db, "tok_takeover", "plain-secret-takeover", []string{AbilityRead, AbilityWriteSensitive, AbilityDeploy}, pcs)
}
