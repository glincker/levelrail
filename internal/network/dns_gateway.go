package network

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Coded reasons a gateway listener is not serving, stable for logs and doctor.
const (
	GatewayReasonInUse       = "dns_bind_in_use"
	GatewayReasonDenied      = "dns_bind_denied"
	GatewayReasonFailed      = "dns_bind_failed"
	GatewayReasonBridgesList = "dns_bridge_list_failed"
)

// GatewayBridge is one Levelrail-managed Docker bridge as the resolver sees it.
type GatewayBridge struct {
	Iface   string
	Gateway netip.Addr
	Subnet  netip.Prefix
}

// GatewayDNSConfig wires the per-bridge resolver; every func is read live.
type GatewayDNSConfig struct {
	Enabled  func() bool
	Bridges  func(ctx context.Context) ([]GatewayBridge, error)
	MeshCIDR func() netip.Prefix
	// Port is 53 in production: Docker only forwards to port 53 of a configured server.
	Port     int
	Resolver *Resolver
	Logger   *slog.Logger
	// BindTimeout bounds how long one listener may take to come up.
	BindTimeout time.Duration
}

// GatewayListener is the state of one bridge gateway listener.
type GatewayListener struct {
	Addr   string `json:"addr"`
	Iface  string `json:"iface"`
	Bound  bool   `json:"bound"`
	Code   string `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// GatewayReport is the resolver's view for the doctor check.
type GatewayReport struct {
	Enabled   bool              `json:"enabled"`
	Listeners []GatewayListener `json:"listeners"`
}

// GatewayDNS keeps one mesh DNS server bound to each managed bridge gateway on :53.
type GatewayDNS struct {
	cfg  GatewayDNSConfig
	mu   sync.Mutex
	srv  map[netip.Addr]*DNSServer
	fail map[netip.Addr]GatewayListener
	last map[netip.Addr]string
	nets []netip.Prefix
	qlog *QueryLog
	ifs  map[netip.Addr]string
}

// NewGatewayDNS builds the manager; nothing binds until Reconcile runs.
func NewGatewayDNS(cfg GatewayDNSConfig) *GatewayDNS {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &GatewayDNS{
		cfg: cfg, srv: map[netip.Addr]*DNSServer{}, fail: map[netip.Addr]GatewayListener{},
		last: map[netip.Addr]string{}, ifs: map[netip.Addr]string{},
	}
}

// SetQueryLog attaches the shared query log to every current and future listener.
func (g *GatewayDNS) SetQueryLog(l *QueryLog) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.qlog = l
	for _, s := range g.srv {
		s.SetQueryLog(l)
	}
}

// Confirmed reports whether addr has a listener that is serving right now.
func (g *GatewayDNS) Confirmed(addr netip.Addr) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.srv[addr]
	return ok
}

// Interfaces lists the managed bridge interfaces seen at the last reconcile.
func (g *GatewayDNS) Interfaces() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, 0, len(g.ifs))
	for _, name := range g.ifs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Report returns the listener states, sorted by address.
func (g *GatewayDNS) Report() GatewayReport {
	g.mu.Lock()
	defer g.mu.Unlock()
	rep := GatewayReport{Enabled: g.cfg.Enabled != nil && g.cfg.Enabled()}
	for a, s := range g.srv {
		rep.Listeners = append(rep.Listeners, GatewayListener{Addr: s.Addr().String(), Iface: g.ifs[a], Bound: true})
	}
	for _, f := range g.fail {
		rep.Listeners = append(rep.Listeners, f)
	}
	sort.Slice(rep.Listeners, func(i, j int) bool { return rep.Listeners[i].Addr < rep.Listeners[j].Addr })
	return rep
}

// allowed is the open-resolver guard: container subnets and the mesh only.
func (g *GatewayDNS) allowed(src netip.Addr) bool {
	g.mu.Lock()
	nets := g.nets
	g.mu.Unlock()
	for _, p := range nets {
		if p.Contains(src) {
			return true
		}
	}
	if g.cfg.MeshCIDR != nil {
		if m := g.cfg.MeshCIDR(); m.IsValid() && m.Contains(src) {
			return true
		}
	}
	return false
}

// Reconcile binds missing gateway listeners, drops stale ones, and closes all when disabled.
func (g *GatewayDNS) Reconcile(ctx context.Context) {
	if g.cfg.Enabled == nil || !g.cfg.Enabled() {
		g.closeAll()
		return
	}
	bridges, err := g.cfg.Bridges(ctx)
	if err != nil {
		g.cfg.Logger.Warn("mesh dns: could not list bridges, keeping current listeners",
			slog.String("reason", GatewayReasonBridgesList), slog.String("error", err.Error()))
		return
	}
	want := map[netip.Addr]GatewayBridge{}
	var nets []netip.Prefix
	for _, b := range bridges {
		if b.Gateway.IsValid() {
			want[b.Gateway] = b
		}
		if b.Subnet.IsValid() {
			nets = append(nets, b.Subnet)
		}
	}
	g.mu.Lock()
	g.nets = nets
	g.ifs = map[netip.Addr]string{}
	for a, b := range want {
		g.ifs[a] = b.Iface
	}
	var stale []*DNSServer
	for a, s := range g.srv {
		if _, ok := want[a]; !ok {
			stale = append(stale, s)
			delete(g.srv, a)
		}
	}
	for a := range g.fail {
		if _, ok := want[a]; !ok {
			delete(g.fail, a)
		}
	}
	g.mu.Unlock()
	for _, s := range stale {
		_ = s.Close()
	}
	for a, b := range want {
		g.mu.Lock()
		_, up := g.srv[a]
		g.mu.Unlock()
		if !up {
			g.bind(ctx, a, b)
		}
	}
}

func (g *GatewayDNS) bind(ctx context.Context, gw netip.Addr, b GatewayBridge) {
	bctx := ctx
	if g.cfg.BindTimeout > 0 {
		var cancel context.CancelFunc
		bctx, cancel = context.WithTimeout(ctx, g.cfg.BindTimeout)
		defer cancel()
	}
	srv := NewDNSServer(g.cfg.Resolver, g.cfg.Logger)
	srv.SetAccess(g.allowed)
	addr := net.JoinHostPort(gw.String(), strconv.Itoa(g.cfg.Port))
	if err := srv.Start(bctx, addr); err != nil {
		code := classifyBindError(err)
		g.mu.Lock()
		g.fail[gw] = GatewayListener{Addr: addr, Iface: b.Iface, Code: code, Detail: err.Error()}
		changed := g.last[gw] != code
		g.last[gw] = code
		g.mu.Unlock()
		if changed {
			g.cfg.Logger.Warn("mesh dns: gateway listener not started, containers keep Docker's resolver",
				slog.String("reason", code), slog.String("addr", addr), slog.String("error", err.Error()))
		}
		_ = srv.Close()
		return
	}
	g.mu.Lock()
	srv.SetQueryLog(g.qlog)
	g.srv[gw] = srv
	delete(g.fail, gw)
	delete(g.last, gw)
	g.mu.Unlock()
}

func classifyBindError(err error) string {
	switch {
	case errors.Is(err, syscall.EADDRINUSE):
		return GatewayReasonInUse
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return GatewayReasonDenied
	}
	return GatewayReasonFailed
}

func (g *GatewayDNS) closeAll() {
	g.mu.Lock()
	servers := g.srv
	g.srv = map[netip.Addr]*DNSServer{}
	g.fail = map[netip.Addr]GatewayListener{}
	g.last = map[netip.Addr]string{}
	g.ifs = map[netip.Addr]string{}
	g.mu.Unlock()
	for _, s := range servers {
		_ = s.Close()
	}
}

// Close stops every gateway listener.
func (g *GatewayDNS) Close() { g.closeAll() }
