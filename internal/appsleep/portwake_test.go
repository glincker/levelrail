package appsleep

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type pwStore struct {
	*fakeSleep
	mu      sync.Mutex
	wakeErr map[string]string
}

func (p *pwStore) SetAppSleepWakeError(_ context.Context, n, msg string, _ time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wakeErr[n] = msg
	return nil
}

func (p *pwStore) grafanaErr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.wakeErr["grafana"]
}

// pwApps is a fake app store whose "container" is a loopback echo server that
// appears on the app's host port after it is unsuspended.
type pwApps struct {
	mu        sync.Mutex
	svc       store.DesiredService
	starts    atomic.Int32
	onStart   func(port int)
	startFail error
}

func (a *pwApps) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.svc
	return &c, nil
}

func (a *pwApps) UpdateServiceSuspended(_ context.Context, _ string, suspended bool) error {
	if a.startFail != nil {
		return a.startFail
	}
	a.mu.Lock()
	a.svc.Suspended = suspended
	port := *a.svc.HostPort
	a.mu.Unlock()
	if !suspended {
		a.starts.Add(1)
		if a.onStart != nil {
			a.onStart(port)
		}
	}
	return nil
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func echoServer(t *testing.T, port int) {
	t.Helper()
	var ln net.Listener
	var err error
	for i := 0; i < 50; i++ {
		if ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Errorf("echo server: %v", err)
		return
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
}

type pwHarness struct {
	m     *PortWakeManager
	apps  *pwApps
	st    *pwStore
	ev    *recSleepEvents
	port  int
	close func()
}

func newPW(t *testing.T, bind string, mutate func(*PortWakeManager, *pwApps)) *pwHarness {
	t.Helper()
	port := freePort(t)
	svc := store.DesiredService{Name: "grafana", HostPort: &port, BindAddress: bind, Suspended: true}
	apps := &pwApps{svc: svc}
	apps.onStart = func(p int) { echoServer(t, p) }
	st := &pwStore{fakeSleep: &fakeSleep{rows: map[string]*store.AppSleep{"grafana": {ServiceName: "grafana", IdleMinutes: 30, Sleeping: true, PortWake: true, Since: time.Now().Add(-time.Hour)}}}, wakeErr: map[string]string{}}
	ev := &recSleepEvents{}
	m := &PortWakeManager{
		Store: st, Apps: apps, Events: ev, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Cfg: Config{PortWakeTimeout: 2 * time.Second, PortWakeRetry: 20 * time.Millisecond, PortWakeCollect: 150 * time.Millisecond, PortWakeMinPort: 1024, WakeCooldown: time.Minute, NetworkFloorBytesPerMin: 4096},
	}
	if mutate != nil {
		mutate(m, apps)
	}
	h := &pwHarness{m: m, apps: apps, st: st, ev: ev, port: port}
	t.Cleanup(m.closeAll)
	return h
}

func (h *pwHarness) addr() string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(h.port)) }

func roundTrip(t *testing.T, addr, msg string) error {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		return err
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(c, buf); err != nil {
		return err
	}
	if string(buf) != msg {
		return errors.New("echo mismatch")
	}
	return nil
}

func TestPortWakeSingleFlightWith50Connections(t *testing.T) {
	h := newPW(t, "", nil)
	h.m.Sync(context.Background())
	if st := h.m.Status("grafana"); !st.Active {
		t.Fatalf("no listener after sync: %+v", st)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- roundTrip(t, h.addr(), "hello-"+strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	close(errs)
	failed := 0
	for err := range errs {
		if err != nil {
			failed++
		}
	}
	if h.apps.starts.Load() != 1 {
		t.Fatalf("app started %d times, want exactly 1", h.apps.starts.Load())
	}
	if failed != 0 {
		t.Fatalf("%d of 50 connections failed", failed)
	}
	var woke int
	for _, e := range h.ev.events {
		if e.Kind == store.SleepEventWoke && e.Reason == store.SleepReasonPort {
			woke++
		}
	}
	if woke != 1 {
		t.Errorf("woke events = %d, want 1: %+v", woke, h.ev.events)
	}
	if h.st.rows["grafana"].Sleeping {
		t.Error("sleeping flag still set after the wake")
	}
}

func TestPortWakeHoldsFirstConnectionUntilReady(t *testing.T) {
	h := newPW(t, "", func(_ *PortWakeManager, a *pwApps) {
		a.onStart = func(p int) { go func() { time.Sleep(300 * time.Millisecond); echoServer(t, p) }() }
	})
	h.m.Sync(context.Background())
	begin := time.Now()
	if err := roundTrip(t, h.addr(), "slow-start"); err != nil {
		t.Fatalf("held connection failed: %v", err)
	}
	if time.Since(begin) < 250*time.Millisecond {
		t.Error("connection was answered before the app could have started")
	}
}

func TestPortWakeTimeoutRecordsFailureAndError(t *testing.T) {
	h := newPW(t, "", func(m *PortWakeManager, a *pwApps) {
		a.onStart = func(int) {}
		m.Cfg.PortWakeTimeout = 300 * time.Millisecond
	})
	h.m.Sync(context.Background())
	if err := roundTrip(t, h.addr(), "x"); err == nil {
		t.Fatal("connection to an app that never starts must fail")
	}
	deadline := time.Now().Add(2 * time.Second)
	for h.st.grafanaErr() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.st.grafanaErr() == "" {
		t.Fatal("wake error was not recorded for the app")
	}
	var failedEvents int
	for _, e := range h.ev.events {
		if e.Kind == store.SleepEventWakeFailed && e.Reason == store.SleepReasonPort && e.Detail != "" {
			failedEvents++
		}
	}
	if failedEvents != 1 {
		t.Errorf("wake_failed events = %d: %+v", failedEvents, h.ev.events)
	}
	if h.st.rows["grafana"].Sleeping {
		t.Error("half-succeeded wake: the container was started, so the sleeping flag must be clear and match desired state")
	}
	if h.apps.svc.Suspended {
		t.Error("desired state must say running after the start succeeded")
	}
}

func TestPortWakeAppCrashesDuringWake(t *testing.T) {
	h := newPW(t, "", func(m *PortWakeManager, a *pwApps) {
		m.Cfg.PortWakeTimeout = 400 * time.Millisecond
		a.onStart = func(p int) {
			ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
			if err == nil {
				_ = ln.Close()
			}
		}
	})
	h.m.Sync(context.Background())
	if err := roundTrip(t, h.addr(), "x"); err == nil {
		t.Fatal("a crashed app must not answer")
	}
	deadline := time.Now().Add(2 * time.Second)
	for h.st.grafanaErr() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.st.grafanaErr() == "" {
		t.Error("crash during wake left no visible error")
	}
}

func TestPortWakeStartFailureKeepsAppAsleepAndRebinds(t *testing.T) {
	h := newPW(t, "", func(_ *PortWakeManager, a *pwApps) { a.startFail = errors.New("docker unavailable") })
	h.m.Sync(context.Background())
	_ = roundTrip(t, h.addr(), "x")
	deadline := time.Now().Add(2 * time.Second)
	for h.st.grafanaErr() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.st.grafanaErr() == "" || !h.st.rows["grafana"].Sleeping {
		t.Fatalf("a failed start must keep the app asleep with an error: %q sleeping=%v", h.st.grafanaErr(), h.st.rows["grafana"].Sleeping)
	}
	h.m.Sync(context.Background())
	if !h.m.Status("grafana").Active {
		t.Error("listener must come back so the next connection can retry")
	}
}

func TestPortWakeControlPlaneRestartRebinds(t *testing.T) {
	h := newPW(t, "", nil)
	h.m.Sync(context.Background())
	h.m.closeAll()
	if c, err := net.DialTimeout("tcp", h.addr(), 200*time.Millisecond); err == nil {
		_ = c.Close()
		t.Fatal("listener survived shutdown")
	}
	restarted := &PortWakeManager{Store: h.st, Apps: h.apps, Events: h.ev, Cfg: h.m.Cfg, Logger: h.m.Logger}
	t.Cleanup(restarted.closeAll)
	restarted.Sync(context.Background())
	if !restarted.Status("grafana").Active {
		t.Fatal("a fresh manager must re-establish the listener from stored state")
	}
	if err := roundTrip(t, h.addr(), "after-restart"); err != nil {
		t.Fatalf("wake after restart: %v", err)
	}
}

func TestPortWakeStopsListeningOnceAwake(t *testing.T) {
	h := newPW(t, "", nil)
	h.m.Sync(context.Background())
	h.st.rows["grafana"].Sleeping = false
	h.apps.svc.Suspended = false
	h.m.Sync(context.Background())
	if h.m.Status("grafana").Active {
		t.Error("an awake app must not have a listener on its port")
	}
}

func TestPortWakePortInUseSurfacesAsErrorNotCrash(t *testing.T) {
	h := newPW(t, "", func(m *PortWakeManager, _ *pwApps) { m.Cfg.PortWakeTimeout = 50 * time.Millisecond })
	squatter, err := net.Listen("tcp", h.addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = squatter.Close() }()
	h.m.Sync(context.Background())
	if h.m.Status("grafana").Active {
		t.Fatal("bound a port someone else holds")
	}
	time.Sleep(80 * time.Millisecond)
	h.m.Sync(context.Background())
	st := h.m.Status("grafana")
	if st.Error == "" || st.Active {
		t.Fatalf("status = %+v, want a reported bind error", st)
	}
	if h.st.grafanaErr() == "" {
		t.Error("the error must be stored so the dashboard can show it")
	}
	_ = squatter.Close()
	h.m.Sync(context.Background())
	if st := h.m.Status("grafana"); !st.Active || st.Error != "" {
		t.Errorf("must recover once the port is free: %+v", st)
	}
}

func TestPortWakeBindsExactlyTheAppsInterface(t *testing.T) {
	port := 40123
	tests := []struct {
		bind string
		want string
	}{
		{"", "127.0.0.1:40123"},
		{"private", "127.0.0.1:40123"},
		{"public", "0.0.0.0:40123"},
		{"127.0.0.1", "127.0.0.1:40123"},
		{"::1", "[::1]:40123"},
		{"10.9.0.4", "10.9.0.4:40123"},
	}
	for _, tc := range tests {
		t.Run("bind "+tc.bind, func(t *testing.T) {
			var asked []string
			h := newPW(t, tc.bind, func(m *PortWakeManager, a *pwApps) {
				a.svc.HostPort = &port
				m.Listen = func(_, addr string) (net.Listener, error) {
					asked = append(asked, addr)
					return net.Listen("tcp", "127.0.0.1:0")
				}
			})
			h.m.Sync(context.Background())
			if len(asked) != 1 || asked[0] != tc.want {
				t.Fatalf("listened on %v, want [%s]", asked, tc.want)
			}
		})
	}
}

func TestPortWakeIPv6Loopback(t *testing.T) {
	l, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback on this machine")
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	h := newPW(t, "::1", func(_ *PortWakeManager, a *pwApps) {
		a.svc.HostPort = &port
		a.onStart = func(p int) {
			ln, err := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(p)))
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = ln.Close() })
			go func() {
				for {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
				}
			}()
		}
	})
	h.m.Sync(context.Background())
	if err := roundTrip(t, net.JoinHostPort("::1", strconv.Itoa(port)), "v6"); err != nil {
		t.Fatalf("ipv6 wake: %v", err)
	}
}

func TestPortWakeSupported(t *testing.T) {
	low, ok := 80, 8080
	m := &PortWakeManager{Cfg: Config{PortWakeMinPort: 1024}}
	tests := []struct {
		name string
		svc  store.DesiredService
		want bool
	}{
		{"pinned user port", store.DesiredService{HostPort: &ok}, true},
		{"privileged port", store.DesiredService{HostPort: &low}, false},
		{"no pin", store.DesiredService{}, false},
		{"remote node", store.DesiredService{HostPort: &ok, NodeID: "node-2"}, false},
		{"bad bind address", store.DesiredService{HostPort: &ok, BindAddress: "everywhere"}, false},
	}
	for _, tc := range tests {
		svc := tc.svc
		if got, why := m.Supported(&svc); got != tc.want || (!got && why == "") {
			t.Errorf("%s: supported=%v why=%q", tc.name, got, why)
		}
	}
}
