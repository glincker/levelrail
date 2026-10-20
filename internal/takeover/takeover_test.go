package takeover

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type memStore struct {
	mu   sync.Mutex
	runs map[string]Run
	seq  []string
}

func newMemStore() *memStore { return &memStore{runs: map[string]Run{}} }

func (m *memStore) Create(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[r.ID] = r
	m.seq = append(m.seq, r.ID)
	return nil
}

func (m *memStore) Get(_ context.Context, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return r, nil
}

func (m *memStore) Save(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[r.ID]; !ok {
		return ErrNotFound
	}
	m.runs[r.ID] = r
	return nil
}

func (m *memStore) List(_ context.Context, limit int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for i := len(m.seq) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, m.runs[m.seq[i]])
	}
	return out, nil
}

func (m *memStore) ListByState(_ context.Context, states ...string) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, id := range m.seq {
		if slices.Contains(states, m.runs[id].State) {
			out = append(out, m.runs[id])
		}
	}
	return out, nil
}

type fakeDocker struct {
	running   map[string]bool
	stopErr   map[string]error
	startErrs map[string]int
	calls     []string
}

func newFakeDocker(names ...string) *fakeDocker {
	f := &fakeDocker{running: map[string]bool{}, stopErr: map[string]error{}, startErrs: map[string]int{}}
	for _, n := range names {
		f.running[n] = true
	}
	return f
}

func (f *fakeDocker) Stop(_ context.Context, name string) error {
	f.calls = append(f.calls, "stop "+name)
	if err := f.stopErr[name]; err != nil {
		return err
	}
	f.running[name] = false
	return nil
}

func (f *fakeDocker) Start(_ context.Context, name string) error {
	f.calls = append(f.calls, "start "+name)
	if f.startErrs[name] > 0 {
		f.startErrs[name]--
		return errors.New("port is already allocated")
	}
	f.running[name] = true
	return nil
}

type fakeSettings struct{ cur TLSSettings }

func (f *fakeSettings) GetTLS(context.Context) (TLSSettings, error) { return f.cur, nil }
func (f *fakeSettings) SetTLS(_ context.Context, s TLSSettings) error {
	f.cur = s
	return nil
}

type fakeVerifier struct {
	ok    bool
	calls int
}

func (f *fakeVerifier) Verify(context.Context, string) DomainCheck {
	f.calls++
	if f.ok {
		return DomainCheck{OK: true, Status: 200, Delivered: true}
	}
	return DomainCheck{Detail: "not delivered by this ingress"}
}

type harness struct {
	st       *memStore
	dock     *fakeDocker
	set      *fakeSettings
	ver      *fakeVerifier
	restarts int
	audits   []string
	r        *Runner
}

func newHarness() *harness {
	h := &harness{st: newMemStore(), dock: newFakeDocker("coolify-proxy"), set: &fakeSettings{cur: TLSSettings{TerminatedUpstream: true, PublicHTTPSPort: 8443}}, ver: &fakeVerifier{ok: true}}
	h.r = &Runner{Store: h.st, Cfg: Config{Window: 40 * time.Millisecond, VerifyInterval: 5 * time.Millisecond, StartAttempts: 3, StartWait: time.Millisecond}, Deps: Deps{
		Containers: h.dock, Settings: h.set, Verifier: h.ver,
		Domains: func(context.Context) ([]string, error) { return []string{"a.example.com", "b.example.com"}, nil },
		Restart: func() bool { h.restarts++; return true },
		Audit:   func(_ context.Context, action, _ string) { h.audits = append(h.audits, action) },
	}}
	return h
}

func readyPlan() Plan {
	return Plan{Verdict: VerdictReady, TargetHTTP: ":80", TargetHTTPS: ":443", CurrentHTTP: ":8088", CurrentHTTPS: ":8443", ProxyNames: []string{"coolify-proxy"}}
}

// boot simulates the next process start: PrepareBoot then Resume.
func (h *harness) boot(t *testing.T) BootDecision {
	t.Helper()
	d, err := PrepareBoot(context.Background(), h.st, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	h.r.Resume(context.Background())
	return d
}

func only(t *testing.T, h *harness) Run {
	t.Helper()
	runs, _ := h.st.List(context.Background(), 5)
	if len(runs) != 1 {
		t.Fatalf("want one run, got %d", len(runs))
	}
	return runs[0]
}

func TestHappyPathAppliesVerifiesAndStaysDone(t *testing.T) {
	h := newHarness()
	run, created, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan(), Actor: "root"})
	if err != nil || !created {
		t.Fatalf("start: %v created=%v", err, created)
	}
	if run.State != StateApplying || h.restarts != 1 || h.dock.running["coolify-proxy"] || h.set.cur.TerminatedUpstream {
		t.Fatalf("after start: state=%s restarts=%d running=%v tls=%+v", run.State, h.restarts, h.dock.running, h.set.cur)
	}
	if d := h.boot(t); d.Action != BootApply {
		t.Fatalf("boot decision = %s", d.Action)
	}
	got := only(t, h)
	if got.State != StateDone || got.Boots != 1 || len(got.Verify) != 2 {
		t.Fatalf("after boot: %+v", got)
	}
	if d := h.boot(t); d.Action != BootApply {
		t.Fatalf("a later start of a done run must keep the addresses, got %s", d.Action)
	}
}

func TestStartIsIdempotent(t *testing.T) {
	h := newHarness()
	first, _, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan()})
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan()})
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("repeat start: id %s vs %s created=%v err=%v", second.ID, first.ID, created, err)
	}
	if h.restarts != 1 || len(h.dock.calls) != 1 {
		t.Fatalf("a repeat must not stop or restart again: restarts=%d calls=%v", h.restarts, h.dock.calls)
	}
}

func TestBlockedPlanIsRefused(t *testing.T) {
	h := newHarness()
	p := readyPlan()
	p.Verdict = VerdictBlocked
	if _, _, err := h.r.Start(context.Background(), StartRequest{Plan: p}); !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v", err)
	}
	if len(h.dock.calls) != 0 || h.restarts != 0 {
		t.Fatal("a blocked plan must touch nothing")
	}
}

func TestStopFailureStartsBackWhatWasStopped(t *testing.T) {
	h := newHarness()
	h.dock.running["edge-proxy"] = true
	h.dock.stopErr["edge-proxy"] = errors.New("docker said no")
	p := readyPlan()
	p.ProxyNames = []string{"coolify-proxy", "edge-proxy"}
	run, _, err := h.r.Start(context.Background(), StartRequest{Plan: p})
	if err == nil {
		t.Fatal("want an error")
	}
	if run.State != StateRolledBack || !h.dock.running["coolify-proxy"] || h.restarts != 0 {
		t.Fatalf("state=%s running=%v restarts=%d", run.State, h.dock.running, h.restarts)
	}
	if h.set.cur != (TLSSettings{TerminatedUpstream: true, PublicHTTPSPort: 8443}) {
		t.Fatalf("settings were not left alone: %+v", h.set.cur)
	}
}

func TestBindFailureRollsBackAndRestartsOldProxy(t *testing.T) {
	h := newHarness()
	if _, _, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan()}); err != nil {
		t.Fatal(err)
	}
	// First boot died before verification (could not bind): the second boot must not apply the override.
	if _, err := PrepareBoot(context.Background(), h.st, time.Now()); err != nil {
		t.Fatal(err)
	}
	d, err := PrepareBoot(context.Background(), h.st, time.Now())
	if err != nil || d.Action != BootRollback {
		t.Fatalf("second boot: %s %v", d.Action, err)
	}
	h.dock.startErrs["coolify-proxy"] = 2
	h.r.Resume(context.Background())
	got := only(t, h)
	if got.State != StateRolledBack || !h.dock.running["coolify-proxy"] {
		t.Fatalf("state=%s running=%v err=%s", got.State, h.dock.running, got.Error)
	}
	if h.set.cur != (TLSSettings{TerminatedUpstream: true, PublicHTTPSPort: 8443}) {
		t.Fatalf("tls settings not restored: %+v", h.set.cur)
	}
	if got.Error == "" {
		t.Fatal("the rollback must say why it happened")
	}
}

func TestFailedVerificationRollsBackWithinWindow(t *testing.T) {
	h := newHarness()
	h.ver.ok = false
	if _, _, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan()}); err != nil {
		t.Fatal(err)
	}
	h.boot(t)
	got := only(t, h)
	if got.State != StateRollingBack || h.restarts != 2 {
		t.Fatalf("state=%s restarts=%d", got.State, h.restarts)
	}
	if h.ver.calls < 2 {
		t.Fatalf("expected repeated verification inside the window, got %d", h.ver.calls)
	}
	if d := h.boot(t); d.Action != BootNone {
		t.Fatalf("a rolling back run applies no override, got %s", d.Action)
	}
	got = only(t, h)
	if got.State != StateRolledBack || !h.dock.running["coolify-proxy"] {
		t.Fatalf("after second boot: %s running=%v", got.State, h.dock.running)
	}
}

func TestManualRollbackOfDoneRun(t *testing.T) {
	h := newHarness()
	if _, _, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan()}); err != nil {
		t.Fatal(err)
	}
	h.boot(t)
	id := only(t, h).ID
	run, err := h.r.Rollback(context.Background(), id, "operator asked", "root")
	if err != nil || run.State != StateRollingBack || h.restarts != 2 {
		t.Fatalf("rollback: %v state=%s restarts=%d", err, run.State, h.restarts)
	}
	again, err := h.r.Rollback(context.Background(), id, "again", "root")
	if err != nil || again.State != StateRollingBack || h.restarts != 2 {
		t.Fatalf("repeat rollback must be a no-op: %v restarts=%d", err, h.restarts)
	}
	h.boot(t)
	if got := only(t, h); got.State != StateRolledBack {
		t.Fatalf("state = %s", got.State)
	}
	if _, err := h.r.Rollback(context.Background(), id, "late", "root"); err != nil {
		t.Fatalf("rollback of a rolled back run: %v", err)
	}
	if !slices.Contains(h.audits, "takeover.rollback") || !slices.Contains(h.audits, "takeover.start_proxy") {
		t.Fatalf("audits = %v", h.audits)
	}
}

func TestOldProxyThatWillNotStartIsReportedFailed(t *testing.T) {
	h := newHarness()
	if _, _, err := h.r.Start(context.Background(), StartRequest{Plan: readyPlan()}); err != nil {
		t.Fatal(err)
	}
	h.boot(t)
	id := only(t, h).ID
	if _, err := h.r.Rollback(context.Background(), id, "undo", "root"); err != nil {
		t.Fatal(err)
	}
	h.dock.startErrs["coolify-proxy"] = 99
	h.boot(t)
	got := only(t, h)
	if got.State != StateFailed || got.Error == "" {
		t.Fatalf("state=%s err=%q", got.State, got.Error)
	}
}

func TestDecideBoot(t *testing.T) {
	tests := []struct {
		name string
		run  Run
		want string
	}{
		{"preparing rolls back", Run{State: StatePreparing}, BootRollback},
		{"first applying boot applies", Run{State: StateApplying}, BootApply},
		{"second applying boot rolls back", Run{State: StateApplying, Boots: 1}, BootRollback},
		{"restart mid verification rolls back", Run{State: StateVerifying}, BootRollback},
		{"done keeps the override", Run{State: StateDone}, BootApply},
		{"rolling back applies nothing", Run{State: StateRollingBack}, BootNone},
		{"rolled back applies nothing", Run{State: StateRolledBack}, BootNone},
		{"failed applies nothing", Run{State: StateFailed}, BootNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideBoot(tt.run).Action; got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}

func TestTraefikHosts(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{"single", map[string]string{"traefik.http.routers.a.rule": "Host(`App.Example.com`)"}, []string{"app.example.com"}},
		{"several and path", map[string]string{"traefik.http.routers.a.rule": "Host(`a.example.com`) && PathPrefix(`/x`) || Host(`b.example.com`, `c.example.com`)"},
			[]string{"a.example.com", "b.example.com", "c.example.com"}},
		{"regex skipped", map[string]string{"traefik.http.routers.a.rule": "HostRegexp(`{sub:.+}.example.com`)"}, []string{}},
		{"other labels", map[string]string{"coolify.name": "x", "traefik.http.services.a.loadbalancer.server.port": "80"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TraefikHosts(tt.labels); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func healthyFacts() Facts {
	return Facts{
		Holders:      []Holder{{Port: 80, Container: "edge-proxy", Image: "traefik:v3", Kind: "traefik", RestartPolicy: "unless-stopped"}, {Port: 443, Container: "edge-proxy", Image: "traefik:v3", Kind: "traefik"}},
		CurrentHTTP:  ":8088",
		CurrentHTTPS: ":8443",
		Domains: []DomainFact{
			{Domain: "a.example.com", App: "a", Healthy: true, ResolvesTo: []string{"203.0.113.9"}, CustomCert: true},
			{Domain: "b.example.com", App: "b", Healthy: true, ResolvesTo: []string{"203.0.113.9"}, CustomCert: true},
		},
		ServerIPs: []string{"203.0.113.9"}, ACMEEnabled: true, DiskFreeGB: 40, DiskKnown: true, MemoryMB: 4096, MinDiskGB: 2, MinMemoryMB: 256,
		RestartSupported: true, DockerAvailable: true, Now: time.Unix(0, 0),
	}
}

func checkStatus(p Plan, id string) string {
	for _, c := range p.Checks {
		if c.ID == id {
			return c.Status
		}
	}
	return "missing"
}

func TestBuildPlan(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Facts)
		verdict string
		checks  map[string]string
	}{
		{"all good with custom certs", func(*Facts) {}, VerdictReady, map[string]string{CheckPorts: StatusPass, CheckHealth: StatusPass, CheckDNS: StatusPass, CheckCerts: StatusPass}},
		{"acme certs warn about http-01", func(f *Facts) {
			f.Domains[0].CustomCert = false
		}, VerdictWarn, map[string]string{CheckCerts: StatusWarn, CheckACME: StatusPass}},
		{"acme off with acme domains blocks", func(f *Facts) {
			f.Domains[0].CustomCert = false
			f.ACMEEnabled = false
		}, VerdictBlocked, map[string]string{CheckACME: StatusBlock}},
		{"host service holds the port", func(f *Facts) {
			f.Holders = nil
			f.HostListeners = []int{80}
		}, VerdictBlocked, map[string]string{CheckPorts: StatusBlock}},
		{"unknown container holds the port", func(f *Facts) {
			f.Holders = append(f.Holders, Holder{Port: 443, Container: "mystery", Image: "foo/bar"})
		}, VerdictBlocked, map[string]string{CheckPorts: StatusBlock}},
		{"unhealthy app", func(f *Facts) {
			f.Domains[1].Healthy = false
		}, VerdictBlocked, map[string]string{CheckHealth: StatusBlock}},
		{"no domains", func(f *Facts) {
			f.Domains = nil
		}, VerdictBlocked, map[string]string{CheckHealth: StatusBlock}},
		{"domain not resolving", func(f *Facts) {
			f.Domains[0].ResolvesTo = nil
			f.Domains[0].DNSErr = "no such host"
		}, VerdictBlocked, map[string]string{CheckDNS: StatusBlock}},
		{"domain resolves elsewhere", func(f *Facts) {
			f.Domains[0].ResolvesTo = []string{"198.51.100.4"}
		}, VerdictWarn, map[string]string{CheckDNS: StatusWarn}},
		{"two copies of one domain", func(f *Facts) {
			f.LabelRouters = []LabelRouter{{Domain: "a.example.com", Container: "old-a"}}
		}, VerdictWarn, map[string]string{CheckConflict: StatusWarn}},
		{"old domain not managed blocks", func(f *Facts) {
			f.OldDomains = []string{"a.example.com", "legacy.example.com"}
		}, VerdictBlocked, map[string]string{CheckUnmanaged: StatusBlock}},
		{"old domain not managed accepted", func(f *Facts) {
			f.OldDomains = []string{"legacy.example.com"}
			f.AcceptUnmanaged = true
		}, VerdictWarn, map[string]string{CheckUnmanaged: StatusWarn}},
		{"low disk", func(f *Facts) { f.DiskFreeGB = 0.2 }, VerdictBlocked, map[string]string{CheckResources: StatusBlock}},
		{"disk unknown", func(f *Facts) { f.DiskKnown = false }, VerdictWarn, map[string]string{CheckResources: StatusWarn}},
		{"no supervisor", func(f *Facts) { f.RestartSupported = false }, VerdictBlocked, map[string]string{CheckRestart: StatusBlock}},
		{"no docker", func(f *Facts) { f.DockerAvailable = false }, VerdictBlocked, map[string]string{CheckDocker: StatusBlock}},
		{"nginx is saved not enforced", func(f *Facts) {
			f.Holders = []Holder{{Port: 80, Container: "web", Image: "nginx", Kind: "nginx"}}
		}, VerdictWarn, map[string]string{CheckSnippet: StatusWarn}},
		{"coolify proxy may be restarted", func(f *Facts) {
			f.Holders[0].RestartPolicy = "always"
			f.Holders[0].Container = "coolify-proxy"
		}, VerdictWarn, map[string]string{CheckPlatform: StatusWarn, CheckRestartPol: StatusWarn}},
		{"systemd socket activation is called out", func(f *Facts) { f.SocketActivation = true }, VerdictWarn, map[string]string{CheckSocket: StatusWarn}},
		{"already listening on 80 and 443", func(f *Facts) {
			f.CurrentHTTP, f.CurrentHTTPS = ":80", ":443"
		}, VerdictDone, map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := healthyFacts()
			tt.mutate(&f)
			p := BuildPlan(f)
			if p.Verdict != tt.verdict {
				t.Fatalf("verdict = %s, want %s: %+v", p.Verdict, tt.verdict, p.Checks)
			}
			for id, want := range tt.checks {
				if got := checkStatus(p, id); got != want {
					t.Errorf("check %s = %s, want %s", id, got, want)
				}
			}
		})
	}
}

func TestPlanListsStepsAndHonestRollback(t *testing.T) {
	p := BuildPlan(healthyFacts())
	var ids []string
	for _, s := range p.Steps {
		ids = append(ids, s.ID)
	}
	want := []string{StepRollback, StepStopProxy, StepTLS, StepListen, StepRestart, StepVerify}
	if !slices.Equal(ids, want) {
		t.Fatalf("steps = %v", ids)
	}
	if len(p.Rollback) == 0 || p.WindowSeconds != int(DefaultWindow.Seconds()) || p.TargetHTTP != ":80" {
		t.Fatalf("plan = %+v", p)
	}
	if !slices.Equal(p.ProxyNames, []string{"edge-proxy"}) || p.ProxyKind != "traefik" {
		t.Fatalf("proxy = %v %s", p.ProxyNames, p.ProxyKind)
	}
}
