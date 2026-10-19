package appsleep

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// portWakePoll is how often a woken app is probed for readiness.
var portWakePoll = 100 * time.Millisecond

// PortWakeStore is the sleep settings surface port wake reads and writes.
type PortWakeStore interface {
	SleepStore
	GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
	SetAppSleepWakeError(ctx context.Context, serviceName, msg string, now time.Time) error
}

type latencyStore interface {
	SetSleepEventLatency(ctx context.Context, id int64, ms int64) error
}

type releasedStore interface {
	SetSleepEventReleased(ctx context.Context, id, ms int64) error
}

// PortWakeManager holds a sleeping app's pinned ports and wakes it on the first
// connection. The listener is released before the start so Docker can publish
// the port; Sync rebuilds listeners from stored state, so restarts lose nothing.
type PortWakeManager struct {
	Store  PortWakeStore
	Apps   Apps
	Nudger Nudger
	Events EventStore
	Logger *slog.Logger
	Cfg    Config
	Now    func() time.Time
	// IsLocal says whether nodeID is the node this process runs on; nil treats "" as local.
	IsLocal func(nodeID string) bool
	// Ready adds an application level readiness check on top of the port answering.
	Ready func(ctx context.Context, name string, since time.Time) bool
	// Listen and Dial are replaceable for tests.
	Listen func(network, addr string) (net.Listener, error)
	Dial   func(ctx context.Context, network, addr string) (net.Conn, error)

	mu      sync.Mutex
	apps    map[string]*portApp
	proxied atomic.Int64
}

type portApp struct {
	name      string
	addr      string
	ports     []int
	lns       []net.Listener
	loops     sync.WaitGroup
	waiting   []net.Conn
	waking    bool
	failSince time.Time
	err       string
	conns     atomic.Int64
}

func (m *PortWakeManager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *PortWakeManager) cfg() Config {
	if m.Cfg == (Config{}) {
		return ConfigFromEnv(nil)
	}
	return m.Cfg
}

func (m *PortWakeManager) local(nodeID string) bool {
	if m.IsLocal != nil {
		return m.IsLocal(nodeID)
	}
	return nodeID == ""
}

// Supported reports whether svc's pinned ports can be held while it sleeps.
func (m *PortWakeManager) Supported(svc *store.DesiredService) (bool, string) {
	ports := PinnedPorts(svc)
	if len(ports) == 0 {
		return false, "the app pins no host port"
	}
	if !m.local(svc.NodeID) {
		return false, "the app runs on another server, where the control plane cannot hold its port"
	}
	if _, err := bindaddr.Resolve(svc.BindAddress); err != nil {
		return false, "its bind address is not valid"
	}
	for _, p := range ports {
		if p < m.cfg().PortWakeMinPort || p > 65535 {
			return false, fmt.Sprintf("host port %d is below the lowest port port wake may bind (%d)", p, m.cfg().PortWakeMinPort)
		}
	}
	return true, ""
}

// Status reports the listener state for name.
func (m *PortWakeManager) Status(name string) PortWakeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.apps[name]
	if a == nil {
		return PortWakeStatus{}
	}
	return PortWakeStatus{Active: len(a.lns) > 0, Ports: append([]int(nil), a.ports...), Error: a.err}
}

// Open counts connections to name that this process is proxying plus the
// established ones the kernel reports on the pinned ports.
func (m *PortWakeManager) Open(_ context.Context, name string, ports []int) int {
	n := 0
	m.mu.Lock()
	if a := m.apps[name]; a != nil {
		n += int(a.conns.Load())
	}
	m.mu.Unlock()
	return n + establishedOn(ports)
}

// Run syncs every retry interval until ctx is done.
func (m *PortWakeManager) Run(ctx context.Context) error {
	m.Sync(ctx)
	t := time.NewTicker(m.cfg().PortWakeRetry)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.closeAll()
			return ctx.Err()
		case <-t.C:
			m.Sync(ctx)
		}
	}
}

type want struct {
	svc  *store.DesiredService
	addr string
}

// Sync makes the listeners match the sleeping apps that use port wake.
func (m *PortWakeManager) Sync(ctx context.Context) {
	rows, err := m.Store.ListAppSleep(ctx)
	if err != nil {
		m.logger().Warn("appsleep: port wake sync: list settings failed", slog.String("error", err.Error()))
		return
	}
	desired := map[string]want{}
	for _, r := range rows {
		if !r.Sleeping || !r.PortWake {
			continue
		}
		svc, err := m.Apps.GetDesiredService(ctx, r.ServiceName)
		if err != nil || !svc.Suspended {
			continue
		}
		if ok, _ := m.Supported(svc); !ok {
			continue
		}
		ip, _ := bindaddr.Resolve(svc.BindAddress)
		desired[r.ServiceName] = want{svc: svc, addr: ip}
	}
	m.mu.Lock()
	for name, a := range m.apps {
		if _, ok := desired[name]; !ok && !a.waking {
			m.dropLocked(a)
			delete(m.apps, name)
		}
	}
	if m.apps == nil {
		m.apps = map[string]*portApp{}
	}
	var pending []*portApp
	for name, w := range desired {
		a := m.apps[name]
		if a == nil {
			a = &portApp{name: name, addr: w.addr, ports: PinnedPorts(w.svc)}
			m.apps[name] = a
		}
		if !a.waking && len(a.lns) == 0 {
			pending = append(pending, a)
		}
	}
	m.mu.Unlock()
	for _, a := range pending {
		m.bind(ctx, a)
	}
}

func (m *PortWakeManager) logger() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

func (m *PortWakeManager) listen(network, addr string) (net.Listener, error) {
	if m.Listen != nil {
		return m.Listen(network, addr)
	}
	return (&net.ListenConfig{}).Listen(context.Background(), network, addr)
}

// bind opens every pinned port on exactly the app's own bind address. A
// failure is retried each Sync and only reported once it outlasts the time a
// stopping container needs to let go of the port.
func (m *PortWakeManager) bind(ctx context.Context, a *portApp) {
	var lns []net.Listener
	var bindErr error
	for _, p := range a.ports {
		ln, err := m.listen("tcp", net.JoinHostPort(a.addr, strconv.Itoa(p)))
		if err != nil {
			bindErr = fmt.Errorf("host port %d: %w", p, err)
			break
		}
		lns = append(lns, ln)
	}
	now := m.now()
	m.mu.Lock()
	if bindErr != nil {
		for _, ln := range lns {
			_ = ln.Close()
		}
		if a.failSince.IsZero() {
			a.failSince = now
		}
		report := now.Sub(a.failSince) >= m.cfg().PortWakeTimeout && a.err == ""
		if report {
			a.err = describePortError(bindErr, a.ports)
		}
		msg := a.err
		m.mu.Unlock()
		if report {
			m.logger().Warn("appsleep: port wake cannot bind", slog.String("app", a.name), slog.String("error", msg))
			if err := m.Store.SetAppSleepWakeError(ctx, a.name, "Port wake could not take the pinned port: "+msg, now); err != nil {
				m.logger().Warn("appsleep: record port wake error failed", slog.String("app", a.name), slog.String("error", err.Error()))
			}
		}
		return
	}
	a.lns, a.failSince, a.err = lns, time.Time{}, ""
	for i, ln := range lns {
		a.loops.Add(1)
		go m.accept(a, ln, a.ports[i])
	}
	m.mu.Unlock()
	m.logger().Info("appsleep: port wake listening", slog.String("app", a.name), slog.String("addr", a.addr))
}

func describePortError(err error, ports []int) string {
	holder := portOwner(ports)
	if holder != "" {
		return fmt.Sprintf("%v (held by %s)", err, holder)
	}
	return err.Error()
}

func (m *PortWakeManager) accept(a *portApp, ln net.Listener, port int) {
	defer a.loops.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		m.mu.Lock()
		a.waiting = append(a.waiting, c)
		start := !a.waking
		a.waking = true
		m.mu.Unlock()
		if start {
			go m.wakeAndServe(a, port)
		}
	}
}

// dropLocked closes a's listeners; the caller holds m.mu.
func (m *PortWakeManager) dropLocked(a *portApp) {
	for _, ln := range a.lns {
		_ = ln.Close()
	}
	a.lns = nil
	for _, c := range a.waiting {
		_ = c.Close()
	}
	a.waiting = nil
}

func (m *PortWakeManager) closeAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.apps {
		m.dropLocked(a)
	}
	m.apps = nil
}

// wakeAndServe is the single flight wake: however many connections arrive,
// the app is started once and every held connection is handed over together.
func (m *PortWakeManager) wakeAndServe(a *portApp, port int) {
	cfg := m.cfg()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.PortWakeTimeout)
	defer cancel()
	// Keep accepting briefly so a burst is held, then release the port: Docker
	// cannot publish it while this process still listens.
	select {
	case <-time.After(cfg.PortWakeCollect):
	case <-ctx.Done():
	}
	m.mu.Lock()
	lns := a.lns
	a.lns = nil
	m.mu.Unlock()
	for _, ln := range lns {
		_ = ln.Close()
	}
	a.loops.Wait()

	start := m.now()
	setting, _ := m.Store.GetAppSleep(ctx, a.name)
	woke, err := Wake(ctx, m.Store, m.Apps, m.Nudger, a.name, start)
	if err != nil {
		m.fail(a, fmt.Errorf("could not start the app: %w", err), false)
		return
	}
	var eventID int64
	if woke {
		e := WokeEvent(a.name, store.SleepReasonPort, "system", setting.Since, start)
		e.TriggerSource = "port " + strconv.Itoa(port)
		eventID = Record(ctx, m.Events, m.Logger, e)
	}
	target := net.JoinHostPort(dialHost(a.addr), strconv.Itoa(port))
	probe, err := m.waitReady(ctx, a.name, target, start)
	if err != nil {
		m.fail(a, err, true)
		return
	}
	if ls, ok := m.Events.(latencyStore); ok && eventID != 0 {
		ms := m.now().Sub(start).Milliseconds()
		_ = ls.SetSleepEventLatency(context.Background(), eventID, ms)
		if rs, ok := m.Events.(releasedStore); ok {
			_ = rs.SetSleepEventReleased(context.Background(), eventID, ms)
		}
	}
	m.mu.Lock()
	waiting := a.waiting
	a.waiting, a.waking = nil, false
	m.mu.Unlock()
	for i, c := range waiting {
		up := probe
		probe = nil
		if i > 0 || up == nil {
			dctx, dcancel := context.WithTimeout(context.Background(), cfg.PortWakeTimeout)
			var derr error
			up, derr = m.dial(dctx, target)
			dcancel()
			if derr != nil {
				_ = c.Close()
				continue
			}
		}
		go m.pipe(a, c, up)
	}
	if probe != nil {
		_ = probe.Close()
	}
}

func dialHost(ip string) string {
	switch ip {
	case "0.0.0.0":
		return "127.0.0.1"
	case "::":
		return "::1"
	}
	return ip
}

func (m *PortWakeManager) dial(ctx context.Context, target string) (net.Conn, error) {
	if m.Dial != nil {
		return m.Dial(ctx, "tcp", target)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", target)
}

// waitReady polls until the app answers on target (and passes the Ready hook),
// returning the live connection so the first client reuses it.
func (m *PortWakeManager) waitReady(ctx context.Context, name, target string, since time.Time) (net.Conn, error) {
	t := time.NewTicker(portWakePoll)
	defer t.Stop()
	for {
		if m.Ready == nil || m.Ready(ctx, name, since) {
			c, err := m.dial(ctx, target)
			if err == nil {
				return c, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("the app was not ready within %s", m.cfg().PortWakeTimeout)
		case <-t.C:
		}
	}
}

// fail records an unfinished wake. A started app stays on (the reconciler
// reports why it is unhealthy); otherwise the next Sync re-binds the listeners.
func (m *PortWakeManager) fail(a *portApp, cause error, started bool) {
	m.mu.Lock()
	for _, c := range a.waiting {
		_ = c.Close()
	}
	a.waiting, a.waking = nil, false
	m.mu.Unlock()
	msg := cause.Error()
	m.logger().Warn("appsleep: port wake failed", slog.String("app", a.name), slog.Bool("started", started), slog.String("error", msg))
	Record(context.Background(), m.Events, m.Logger, store.SleepEvent{ServiceName: a.name, Kind: store.SleepEventWakeFailed, At: m.now(), Reason: store.SleepReasonPort, Detail: msg, Actor: "system"})
	if err := m.Store.SetAppSleepWakeError(context.Background(), a.name, "Port wake failed: "+msg, m.now()); err != nil {
		m.logger().Warn("appsleep: record port wake error failed", slog.String("app", a.name), slog.String("error", err.Error()))
	}
}

func (m *PortWakeManager) pipe(a *portApp, client, upstream net.Conn) {
	a.conns.Add(1)
	m.proxied.Add(1)
	defer func() {
		a.conns.Add(-1)
		m.proxied.Add(-1)
	}()
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			m.logger().Debug("appsleep: port wake copy ended", slog.String("app", a.name), slog.String("error", err.Error()))
		}
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(upstream, client)
	go cp(client, upstream)
	<-done
	<-done
	_ = client.Close()
	_ = upstream.Close()
}
