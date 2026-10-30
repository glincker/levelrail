// Package egress is a forward proxy that lets a plugin reach only the
// hosts it declared, and never an internal address.
package egress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Denial reasons passed to the deny callback.
const (
	ReasonUnknownCaller = "unknown_caller"
	ReasonHostNotListed = "host_not_allowed"
	ReasonBlockedAddr   = "blocked_address"
	ReasonResolveFailed = "resolve_failed"
	ReasonDialFailed    = "dial_failed"
	ReasonTunnelCap     = "too_many_tunnels"
	ReasonRateLimit     = "rate_limited"
	ReasonBadRequest    = "bad_request"
)

// Identifier maps a connection's remote address to a plugin name.
type Identifier interface {
	Identify(remoteAddr string) (plugin string, ok bool)
}

// Allowlist says whether a plugin may reach host:port.
type Allowlist interface {
	Allowed(plugin, host string, port int) bool
}

// Resolver looks up the addresses of a host.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// DenyFunc records a refused request. It never receives payload.
type DenyFunc func(plugin, host string, port int, reason string)

// Options wires the proxy. Identifier and Allowlist are required.
type Options struct {
	Identifier Identifier
	Allowlist  Allowlist
	// Resolver defaults to net.DefaultResolver.
	Resolver Resolver
	OnDeny   DenyFunc
	Config   Config
}

// Proxy is an http.Handler serving CONNECT and absolute-form requests.
type Proxy struct {
	id       Identifier
	allow    Allowlist
	resolver Resolver
	onDeny   DenyFunc
	cfg      Config
	now      func() time.Time

	mu      sync.Mutex
	tunnels map[string]int
	windows map[string]*window
	plain   *http.Transport
}

type window struct {
	start time.Time
	n     int64
}

// New returns a Proxy. It panics on missing required options, which is a
// programming error rather than a runtime condition.
func New(o Options) *Proxy {
	if o.Identifier == nil || o.Allowlist == nil {
		panic("egress: Identifier and Allowlist are required")
	}
	p := &Proxy{
		id: o.Identifier, allow: o.Allowlist, resolver: o.Resolver, onDeny: o.OnDeny,
		cfg: o.Config.withDefaults(), now: time.Now,
		tunnels: map[string]int{}, windows: map[string]*window{},
	}
	if p.resolver == nil {
		p.resolver = net.DefaultResolver
	}
	p.plain = &http.Transport{
		DialContext:           p.plainDial,
		ResponseHeaderTimeout: p.cfg.IdleTimeout,
		DisableKeepAlives:     true,
	}
	return p
}

type dialKey struct{}

func (p *Proxy) plainDial(ctx context.Context, _, addr string) (net.Conn, error) {
	host, ps, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(ps)
	conn, reason, err := p.dial(ctx, host, port)
	if err != nil {
		if r, ok := ctx.Value(dialKey{}).(*string); ok {
			*r = reason
		}
		return nil, err
	}
	return conn, nil
}

func (p *Proxy) deny(w http.ResponseWriter, code int, plugin, host string, port int, reason string) {
	if p.onDeny != nil {
		p.onDeny(plugin, host, port, reason)
	}
	http.Error(w, "egress denied: "+reason, code)
}

// ServeHTTP implements http.Handler.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	plugin, ok := p.id.Identify(r.RemoteAddr)
	if !ok {
		p.deny(w, http.StatusForbidden, "", "", 0, ReasonUnknownCaller)
		return
	}
	if r.Method == http.MethodConnect {
		p.serveConnect(w, r, plugin)
		return
	}
	p.servePlain(w, r, plugin)
}

func splitTarget(hostport, defPort string) (string, int, bool) {
	host, ps, err := net.SplitHostPort(hostport)
	if err != nil {
		host, ps = strings.Trim(hostport, "[]"), defPort
	}
	port, err := strconv.Atoi(ps)
	if err != nil || host == "" || port < 1 || port > 65535 {
		return "", 0, false
	}
	return strings.ToLower(strings.TrimSuffix(host, ".")), port, true
}

func (p *Proxy) serveConnect(w http.ResponseWriter, r *http.Request, plugin string) {
	host, port, ok := splitTarget(r.Host, "443")
	if !ok {
		p.deny(w, http.StatusBadRequest, plugin, r.Host, 0, ReasonBadRequest)
		return
	}
	if !p.allow.Allowed(plugin, host, port) {
		p.deny(w, http.StatusForbidden, plugin, host, port, ReasonHostNotListed)
		return
	}
	if !p.acquire(plugin) {
		p.deny(w, http.StatusTooManyRequests, plugin, host, port, ReasonTunnelCap)
		return
	}
	defer p.release(plugin)

	up, reason, err := p.dial(r.Context(), host, port)
	if err != nil {
		code := http.StatusBadGateway
		if reason == ReasonBlockedAddr {
			code = http.StatusForbidden
		}
		p.deny(w, code, plugin, host, port, reason)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = up.Close()
		http.Error(w, "egress denied: "+ReasonBadRequest, http.StatusInternalServerError)
		return
	}
	client, rw, err := hj.Hijack()
	if err != nil {
		_ = up.Close()
		return
	}
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	p.pipe(plugin, client, rw.Reader, up)
}

func (p *Proxy) servePlain(w http.ResponseWriter, r *http.Request, plugin string) {
	if r.URL == nil || r.URL.Scheme != "http" || r.URL.Host == "" {
		p.deny(w, http.StatusBadRequest, plugin, r.Host, 0, ReasonBadRequest)
		return
	}
	host, port, ok := splitTarget(r.URL.Host, "80")
	if !ok {
		p.deny(w, http.StatusBadRequest, plugin, r.URL.Host, 0, ReasonBadRequest)
		return
	}
	if !p.allow.Allowed(plugin, host, port) {
		p.deny(w, http.StatusForbidden, plugin, host, port, ReasonHostNotListed)
		return
	}
	if !p.acquire(plugin) {
		p.deny(w, http.StatusTooManyRequests, plugin, host, port, ReasonTunnelCap)
		return
	}
	defer p.release(plugin)

	var dialReason string
	out := r.Clone(context.WithValue(r.Context(), dialKey{}, &dialReason))
	out.RequestURI = ""
	for _, h := range []string{"Proxy-Connection", "Proxy-Authorization", "Proxy-Authenticate", "Connection", "Keep-Alive", "Te", "Trailer", "Upgrade"} {
		out.Header.Del(h)
	}
	resp, err := p.plain.RoundTrip(out)
	if err != nil {
		reason, code := ReasonDialFailed, http.StatusBadGateway
		if dialReason != "" {
			reason = dialReason
		}
		if reason == ReasonBlockedAddr {
			code = http.StatusForbidden
		}
		p.deny(w, code, plugin, host, port, reason)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if !p.spend(plugin, int64(n)) {
				if p.onDeny != nil {
					p.onDeny(plugin, host, port, ReasonRateLimit)
				}
				return
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// dial resolves host itself and connects to a vetted IP. The Control hook
// repeats the check on the exact address dialed, so a rebinding resolver
// cannot slip an internal address past the first check.
func (p *Proxy) dial(ctx context.Context, host string, port int) (net.Conn, string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.ConnectTimeout)
	defer cancel()

	var addrs []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{ip}
	} else {
		var lerr error
		addrs, lerr = p.resolver.LookupNetIP(ctx, "ip", host)
		if lerr != nil || len(addrs) == 0 {
			return nil, ReasonResolveFailed, errors.New("resolve failed")
		}
	}
	d := net.Dialer{
		Timeout: p.cfg.ConnectTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			return p.checkAddress(address)
		},
	}
	sawAllowed := false
	for _, a := range addrs {
		if !p.cfg.AllowPrivate && netguard.IsBlocked(a) {
			continue
		}
		sawAllowed = true
		c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(a.Unmap().String(), strconv.Itoa(port)))
		if err == nil {
			return c, "", nil
		}
	}
	if !sawAllowed {
		return nil, ReasonBlockedAddr, errors.New("blocked address")
	}
	return nil, ReasonDialFailed, errors.New("dial failed")
}

func (p *Proxy) checkAddress(address string) error {
	if p.cfg.AllowPrivate {
		return nil
	}
	h, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(h)
	if err != nil {
		return err
	}
	if netguard.IsBlocked(ip) {
		return fmt.Errorf("%w: %s", netguard.ErrBlockedAddress, ip)
	}
	return nil
}

func (p *Proxy) acquire(plugin string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tunnels[plugin] >= p.cfg.MaxTunnels {
		return false
	}
	p.tunnels[plugin]++
	return true
}

func (p *Proxy) release(plugin string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tunnels[plugin] <= 1 {
		delete(p.tunnels, plugin)
		return
	}
	p.tunnels[plugin]--
}

// spend charges n bytes to the plugin's current minute window.
func (p *Proxy) spend(plugin string, n int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	w := p.windows[plugin]
	if w == nil || now.Sub(w.start) >= time.Minute {
		w = &window{start: now}
		p.windows[plugin] = w
	}
	w.n += n
	return w.n <= p.cfg.BytesPerMinute
}

func (p *Proxy) pipe(plugin string, client net.Conn, clientR io.Reader, up net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); p.copyHalf(plugin, up, client, clientR) }()
	go func() { defer wg.Done(); p.copyHalf(plugin, client, up, up) }()
	wg.Wait()
	_ = client.Close()
	_ = up.Close()
}

// copyHalf moves bytes one way, refreshing the idle deadline per chunk.
// A clean EOF half-closes the peer; any other end tears both sides down.
func (p *Proxy) copyHalf(plugin string, dst, src net.Conn, r io.Reader) {
	buf := make([]byte, 32<<10)
	for {
		_ = src.SetReadDeadline(time.Now().Add(p.cfg.IdleTimeout))
		n, err := r.Read(buf)
		if n > 0 {
			if !p.spend(plugin, int64(n)) {
				if p.onDeny != nil {
					p.onDeny(plugin, "", 0, ReasonRateLimit)
				}
				_ = dst.Close()
				_ = src.Close()
				return
			}
			_ = dst.SetWriteDeadline(time.Now().Add(p.cfg.IdleTimeout))
			if _, werr := dst.Write(buf[:n]); werr != nil {
				_ = dst.Close()
				_ = src.Close()
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if cw, ok := dst.(interface{ CloseWrite() error }); ok {
					_ = cw.CloseWrite()
					return
				}
			}
			_ = dst.Close()
			_ = src.Close()
			return
		}
	}
}
