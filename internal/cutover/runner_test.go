package cutover

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memStore struct {
	mu   sync.Mutex
	runs map[string]Run
}

func newMemStore() *memStore { return &memStore{runs: map[string]Run{}} }

func (m *memStore) Create(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.runs {
		if x.App == r.App && (x.State == StatePlanning || x.State == StateStarting || x.State == StateVerifying || x.State == StateSwitching) {
			return errors.New("active")
		}
	}
	m.runs[r.ID] = r
	return nil
}

func (m *memStore) Get(_ context.Context, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, errors.New("not found")
	}
	return deepCopy(r), nil
}

func deepCopy(r Run) Run {
	r.Domains = slices.Clone(r.Domains)
	for i := range r.Domains {
		r.Domains[i].Previous = slices.Clone(r.Domains[i].Previous)
	}
	r.Steps = slices.Clone(r.Steps)
	return r
}

func (m *memStore) Save(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[r.ID] = deepCopy(r)
	return nil
}

func (m *memStore) List(_ context.Context, app string, _ int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, r := range m.runs {
		if r.App == app {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memStore) ListByState(_ context.Context, states ...string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, r := range m.runs {
		if slices.Contains(states, r.State) {
			out = append(out, deepCopy(r))
		}
	}
	return out, nil
}

// world is the fake outside: the app, its DNS zone and the proxy.
type world struct {
	mu       sync.Mutex
	plan     Plan
	failures map[string]error
	running  bool
	attached []string
	dns      map[string][]Record
	proxy    map[string]bool
	calls    []string
	applies  int
	verifyOK map[string]bool
}

func newWorld(method string) *world {
	w := &world{failures: map[string]error{}, dns: map[string][]Record{}, proxy: map[string]bool{}, verifyOK: map[string]bool{}}
	old := Record{Name: "shop", Type: "A", Value: "203.0.113.9", TTL: 300}
	w.dns["shop.example.com"] = []Record{old}
	want := &Record{Name: "shop", Type: "A", Value: "198.51.100.4", TTL: 60}
	w.plan = Plan{App: "shop", Verdict: VerdictReady, HealthPath: "/healthz", Domains: []DomainPlan{{
		Domain: "shop.example.com", Method: method, Provider: "fake", Zone: "example.com", Desired: want, Replace: []Record{old}}}}
	w.verifyOK["shop.example.com"] = true
	return w
}

func (w *world) fail(key string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, key)
	return w.failures[key]
}

func (w *world) Plan(context.Context, PlanRequest) (Plan, error) {
	if err := w.fail("plan"); err != nil {
		return Plan{}, err
	}
	return w.plan, nil
}

func (w *world) Route(_ context.Context, _ string, domains []string) error {
	if err := w.fail("route"); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running, w.attached = true, slices.Clone(domains)
	return nil
}

func (w *world) Unroute(context.Context, string, []string) error {
	if err := w.fail("unroute"); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running, w.attached = false, nil
	return nil
}

func (w *world) WaitHealthy(context.Context, string, time.Duration) (bool, string, error) {
	if err := w.fail("healthy"); err != nil {
		return false, err.Error(), nil
	}
	return true, "Healthy", nil
}

func (w *world) Probe(_ context.Context, _, _ string) (ProbeResult, error) {
	if err := w.fail("probe"); err != nil {
		return ProbeResult{Status: 502, Detail: err.Error()}, nil
	}
	return ProbeResult{OK: true, Status: 200}, nil
}

func (w *world) Apply(_ context.Context, domain string) (DNSResult, error) {
	if err := w.fail("dns.apply"); err != nil {
		return DNSResult{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	prev := w.dns[domain]
	d := w.plan.Domains[0].Desired
	w.dns[domain] = []Record{*d}
	w.applies++
	return DNSResult{Applied: d, Previous: prev, Provider: "fake", Zone: "example.com", Message: "record replaced"}, nil
}

func (w *world) Restore(_ context.Context, domain string, previous []Record, _ *Record) error {
	if err := w.fail("dns.restore"); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.dns[domain] = slices.Clone(previous)
	return nil
}

func (w *world) WriteRoute(_ context.Context, domain string) error {
	if err := w.fail("proxy.write"); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.proxy[domain] = true
	return nil
}

func (w *world) RemoveRoute(_ context.Context, domain string) error {
	if err := w.fail("proxy.remove"); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.proxy, domain)
	return nil
}

func (w *world) Verify(_ context.Context, _, domain string) (bool, string, error) {
	if err := w.fail("verify"); err != nil {
		return false, err.Error(), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.verifyOK[domain] {
		return false, "resolves elsewhere", nil
	}
	return true, "ok", nil
}

func (w *world) deps() Deps {
	return Deps{Planner: w, Host: w, Ingress: w, DNS: w, Proxy: w, Verifier: w}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) sleep(_ context.Context, d time.Duration) error {
	c.t = c.t.Add(d)
	return nil
}

func newRunner(st Store, w *world, clk *fakeClock) *Runner {
	return &Runner{Store: st, Deps: w.deps(), Cfg: DefaultConfig(), Now: clk.now, Sleep: clk.sleep}
}

func startRun(t *testing.T, r *Runner, mode string, write bool) Run {
	t.Helper()
	run, err := r.Start(context.Background(), StartRequest{SessionID: "s", SourceID: "src", App: "shop", Mode: mode, DNSWrite: write, AcceptWarn: true})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func stepNames(r Run) []string {
	var out []string
	for _, s := range r.Steps {
		out = append(out, s.Name)
	}
	return out
}

func TestDryRunExercisesEverythingButTheSwitch(t *testing.T) {
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	r := newRunner(newMemStore(), w, clk)
	run := startRun(t, r, ModeDryRun, true)
	got, err := r.Execute(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateReady || got.FinishedAt.IsZero() {
		t.Fatalf("state = %s (%s)", got.State, got.Error)
	}
	if w.applies != 0 || w.dns["shop.example.com"][0].Value != "203.0.113.9" {
		t.Fatalf("a dry run changed DNS: %+v", w.dns)
	}
	if w.running || len(w.attached) != 0 {
		t.Fatal("a dry run left the app running and attached")
	}
	want := []string{StepReadiness, StepRoute, StepHealthy, StepIngress, StepDNSPreview, StepCleanup}
	if !slices.Equal(stepNames(got), want) {
		t.Fatalf("steps = %v, want %v", stepNames(got), want)
	}
	for _, s := range got.Steps {
		if s.State != StepDone {
			t.Fatalf("step %s is %s", s.Name, s.State)
		}
	}
}

func TestDryRunKeepsAlreadyRoutedAppRunning(t *testing.T) {
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	r := newRunner(newMemStore(), w, clk)
	run, err := r.Start(context.Background(), StartRequest{App: "shop", Mode: ModeDryRun, WasRouted: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Execute(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if !w.running {
		t.Fatal("an app that was serving before the run must not be stopped by it")
	}
}

func TestSwitchGoesLiveAndStoresUndoData(t *testing.T) {
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	st := newMemStore()
	r := newRunner(st, w, clk)
	run := startRun(t, r, ModeSwitch, true)
	got, err := r.Execute(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateLive {
		t.Fatalf("state = %s (%s)", got.State, got.Error)
	}
	d := got.Domains[0]
	if !d.Switched || !d.Verified || len(d.Previous) != 1 || d.Previous[0].Value != "203.0.113.9" || d.Applied == nil {
		t.Fatalf("domain run = %+v", d)
	}
	if w.dns["shop.example.com"][0].Value != "198.51.100.4" {
		t.Fatalf("dns = %+v", w.dns)
	}
	back, err := r.Rollback(context.Background(), run.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if back.State != StateRolledBack || w.dns["shop.example.com"][0].Value != "203.0.113.9" || w.running {
		t.Fatalf("rollback: state=%s dns=%+v running=%v", back.State, w.dns, w.running)
	}
	if _, err := r.Rollback(context.Background(), run.ID, ""); err == nil {
		t.Fatal("a second rollback of a rolled back run must be refused")
	}
}

func TestFailureInjectionAtEachStep(t *testing.T) {
	boom := errors.New("injected")
	tests := []struct {
		name      string
		method    string
		failKey   string
		wantState string
		dnsBack   bool
	}{
		{"plan fails", MethodDNS, "plan", StateFailed, true},
		{"route fails", MethodDNS, "route", StateFailed, true},
		{"app never healthy", MethodDNS, "healthy", StateFailed, true},
		{"ingress probe fails", MethodDNS, "probe", StateFailed, true},
		{"dns apply fails", MethodDNS, "dns.apply", StateRolledBack, true},
		{"proxy write fails", MethodProxy, "proxy.write", StateRolledBack, true},
		{"post switch verify fails", MethodDNS, "verify", StateRolledBack, true},
		{"post switch verify fails (proxy)", MethodProxy, "verify", StateRolledBack, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, clk := newWorld(tc.method), &fakeClock{t: time.Unix(1000, 0)}
			w.failures[tc.failKey] = boom
			r := newRunner(newMemStore(), w, clk)
			run := startRun(t, r, ModeSwitch, true)
			got, err := r.Execute(context.Background(), run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.wantState {
				t.Fatalf("state = %s, want %s (%s)", got.State, tc.wantState, got.Error)
			}
			if got.Error == "" {
				t.Fatal("a failed run must say why")
			}
			if w.dns["shop.example.com"][0].Value != "203.0.113.9" {
				t.Fatalf("dns was not left at its previous value: %+v", w.dns)
			}
			if w.running || len(w.attached) != 0 {
				t.Fatal("the staged app must be stopped and detached after a failure")
			}
			if len(w.proxy) != 0 {
				t.Fatalf("a proxy route was left behind: %+v", w.proxy)
			}
		})
	}
}

func TestVerifyFailureRollsBackWithinBoundedTime(t *testing.T) {
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	w.verifyOK["shop.example.com"] = false
	r := newRunner(newMemStore(), w, clk)
	r.Cfg.VerifyTimeout = 2 * time.Minute
	r.Cfg.VerifyInterval = 10 * time.Second
	run := startRun(t, r, ModeSwitch, true)
	start := clk.t
	got, err := r.Execute(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateRolledBack {
		t.Fatalf("state = %s", got.State)
	}
	if elapsed := clk.t.Sub(start); elapsed < 2*time.Minute || elapsed > 3*time.Minute {
		t.Fatalf("verification waited %s, want about the 2m bound", elapsed)
	}
}

func TestRollbackRetriesAfterPartialFailure(t *testing.T) {
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	w.verifyOK["shop.example.com"] = false
	w.failures["dns.restore"] = errors.New("provider down")
	r := newRunner(newMemStore(), w, clk)
	r.Cfg.VerifyTimeout = 20 * time.Second
	run := startRun(t, r, ModeSwitch, true)
	got, _ := r.Execute(context.Background(), run.ID)
	if got.State != StateFailed || !got.Rollbackable() {
		t.Fatalf("state = %s rollbackable=%v (%s)", got.State, got.Rollbackable(), got.Error)
	}
	delete(w.failures, "dns.restore")
	back, err := r.Rollback(context.Background(), run.ID, "retry")
	if err != nil {
		t.Fatal(err)
	}
	if back.State != StateRolledBack || w.dns["shop.example.com"][0].Value != "203.0.113.9" {
		t.Fatalf("state = %s dns = %+v", back.State, w.dns)
	}
}

func TestRestartInTheMiddle(t *testing.T) {
	crash := errors.New("process died")
	for _, method := range []string{MethodDNS, MethodProxy} {
		for crashAt := 1; crashAt < 40; crashAt++ {
			t.Run(fmt.Sprintf("%s/crash-after-save-%d", method, crashAt), func(t *testing.T) {
				w, clk := newWorld(method), &fakeClock{t: time.Unix(1000, 0)}
				st := newMemStore()
				r1 := newRunner(st, w, clk)
				n := 0
				r1.Hook = func(Run, string) error {
					n++
					if n == crashAt {
						return crash
					}
					return nil
				}
				run := startRun(t, r1, ModeSwitch, true)
				_, err := r1.Execute(context.Background(), run.ID)
				if !errors.Is(err, crash) {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				r2 := newRunner(st, w, clk)
				var launched []string
				if err := r2.Recover(context.Background(), func(id string) { launched = append(launched, id) }); err != nil {
					t.Fatal(err)
				}
				if len(launched) != 1 {
					t.Fatalf("launched = %v", launched)
				}
				got, err := r2.Execute(context.Background(), launched[0])
				if err != nil {
					t.Fatal(err)
				}
				if got.State != StateLive {
					t.Fatalf("resumed run ended %s (%s)", got.State, got.Error)
				}
				if method == MethodDNS && w.dns["shop.example.com"][0].Value != "198.51.100.4" {
					t.Fatalf("dns = %+v", w.dns)
				}
				if method == MethodDNS && len(got.Domains[0].Previous) != 1 {
					t.Fatalf("previous value lost across restart: %+v", got.Domains[0])
				}
			})
		}
	}
}

func TestStaleInterruptedRunIsRolledBack(t *testing.T) {
	crash := errors.New("process died")
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	st := newMemStore()
	r1 := newRunner(st, w, clk)
	r1.Hook = func(_ Run, step string) error {
		if step == StepSwitch {
			return crash
		}
		return nil
	}
	run := startRun(t, r1, ModeSwitch, true)
	if _, err := r1.Execute(context.Background(), run.ID); !errors.Is(err, crash) {
		t.Fatalf("err = %v", err)
	}
	if w.dns["shop.example.com"][0].Value != "198.51.100.4" {
		t.Fatal("the switch should have been applied before the crash")
	}
	clk.t = clk.t.Add(2 * time.Hour)
	r2 := newRunner(st, w, clk)
	if err := r2.Recover(context.Background(), func(string) { t.Fatal("a stale run must not resume forward") }); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Get(context.Background(), run.ID)
	if got.State != StateRolledBack || w.dns["shop.example.com"][0].Value != "203.0.113.9" || w.running {
		t.Fatalf("state = %s dns = %+v running = %v", got.State, w.dns, w.running)
	}
}

func TestManualMethodPausesUntilConfirmed(t *testing.T) {
	w, clk := newWorld(MethodManual), &fakeClock{t: time.Unix(1000, 0)}
	r := newRunner(newMemStore(), w, clk)
	run := startRun(t, r, ModeSwitch, false)
	got, err := r.Execute(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateSwitching || got.Awaiting != AwaitingManualDNS || got.Domains[0].Manual == nil {
		t.Fatalf("state = %s awaiting = %q", got.State, got.Awaiting)
	}
	if w.applies != 0 {
		t.Fatal("a manual run must not write DNS")
	}
	live, err := r.ConfirmManual(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if live.State != StateLive {
		t.Fatalf("state = %s (%s)", live.State, live.Error)
	}
}

func TestPlanGates(t *testing.T) {
	t.Run("blocked plan stops before touching the app", func(t *testing.T) {
		w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
		w.plan.Verdict = VerdictBlocked
		w.plan.Checks = []Check{{ID: CheckImage, Status: StatusBlock, Detail: "image missing"}}
		r := newRunner(newMemStore(), w, clk)
		got, _ := r.Execute(context.Background(), startRun(t, r, ModeSwitch, true).ID)
		if got.State != StateFailed || slices.Contains(w.calls, "route") {
			t.Fatalf("state = %s calls = %v", got.State, w.calls)
		}
	})
	t.Run("warnings need acceptance for a switch only", func(t *testing.T) {
		w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
		w.plan.Verdict = VerdictWarn
		w.plan.Checks = []Check{{ID: CheckVolumes, Status: StatusWarn, Detail: "volume pending"}}
		r := newRunner(newMemStore(), w, clk)
		run, _ := r.Start(context.Background(), StartRequest{App: "shop", Mode: ModeSwitch, DNSWrite: true})
		got, _ := r.Execute(context.Background(), run.ID)
		if got.State != StateFailed || slices.Contains(w.calls, "route") {
			t.Fatalf("state = %s", got.State)
		}
		dry, _ := r.Start(context.Background(), StartRequest{App: "shop", Mode: ModeDryRun})
		got, _ = r.Execute(context.Background(), dry.ID)
		if got.State != StateReady {
			t.Fatalf("dry run state = %s (%s)", got.State, got.Error)
		}
	})
	t.Run("one active run per app", func(t *testing.T) {
		w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
		r := newRunner(newMemStore(), w, clk)
		startRun(t, r, ModeSwitch, true)
		if _, err := r.Start(context.Background(), StartRequest{App: "shop", Mode: ModeSwitch}); err == nil {
			t.Fatal("a second active run for the same app must be refused")
		}
	})
}

func TestCancelledContextKeepsTheRunResumable(t *testing.T) {
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	st := newMemStore()
	r := newRunner(st, w, clk)
	run := startRun(t, r, ModeSwitch, true)
	ctx, cancel := context.WithCancel(context.Background())
	r.Hook = func(_ Run, step string) error {
		if step == StepHealthy {
			cancel()
		}
		return nil
	}
	if _, err := r.Execute(ctx, run.ID); err == nil {
		t.Fatal("expected an interruption error")
	}
	got, _ := st.Get(context.Background(), run.ID)
	if got.Terminal() {
		t.Fatalf("a shutdown must not end the run, state = %s", got.State)
	}
	r.Hook = nil
	final, err := r.Execute(context.Background(), run.ID)
	if err != nil || final.State != StateLive {
		t.Fatalf("resume: %v state=%s", err, final.State)
	}
}

// The source platform's API is never given to the runner. The server below
// stands in for it: any request to it fails the test.
func TestSourcePlatformIsNeverContacted(t *testing.T) {
	var hits atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer src.Close()
	w, clk := newWorld(MethodDNS), &fakeClock{t: time.Unix(1000, 0)}
	r := newRunner(newMemStore(), w, clk)
	run := startRun(t, r, ModeSwitch, true)
	if _, err := r.Execute(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Rollback(context.Background(), run.ID, ""); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the source received %d request(s)", hits.Load())
	}
}
