package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/glincker/levelrail/internal/experimental"
)

// Gateway limits are fixed protocol-level safety bounds; the dialer enforces
// its own open timeout through the context it is given.
const (
	tunnelGatewayMaxTotal       = 1024
	tunnelGatewayMaxPerTarget   = 128
	tunnelGatewayOpenTimeout    = 5 * time.Second
	tunnelGatewayIdleTimeout    = 10 * time.Minute
	tunnelGatewayCopyBuffer     = 32 * 1024
	tunnelGatewayErrBodyDrain   = 16 * 1024
	tunnelGatewayErrDrainWindow = 500 * time.Millisecond
	tunnelGatewayMaxSocketPath  = 100
)

var (
	// ErrTunnelGatewayDisabled means the agent-ingress-tunnel flag is off.
	ErrTunnelGatewayDisabled = errors.New("ingress: agent tunnel gateway disabled")
	// ErrTunnelUnsupported means the node's agent cannot carry tunnels.
	ErrTunnelUnsupported = errors.New("ingress: node agent does not support ingress tunnels")
	// ErrTunnelOffline means the node has no live agent session.
	ErrTunnelOffline = errors.New("ingress: node agent is offline")
	// ErrTunnelTimeout means the node did not complete the tunnel handshake in time.
	ErrTunnelTimeout = errors.New("ingress: node agent did not answer in time")
)

// TunnelTarget identifies what a gateway socket reaches. It is set by the
// reconciler and never derived from anything a connection sends.
type TunnelTarget struct {
	NodeID        string
	Service       string
	ContainerPort int
}

// TunnelStream is one open tunnel: a byte stream with half-close.
type TunnelStream interface {
	io.ReadWriteCloser
	CloseWrite() error
}

// TunnelDialer opens a tunnel to a service port on a node. The agent resolves
// the host port itself, so no host or port number travels from here.
type TunnelDialer interface {
	OpenTunnel(ctx context.Context, nodeID, service string, containerPort int) (TunnelStream, error)
}

// TunnelGateway serves one unix socket per target and splices each accepted
// connection to a tunnel. The target is bound to the socket, so a client of
// the public ingress cannot choose the node, service or port.
type TunnelGateway struct {
	dir    string
	dialer TunnelDialer
	logger *slog.Logger

	maxTotal, maxPerTarget int
	openTimeout            time.Duration
	idleTimeout            time.Duration

	mu        sync.Mutex
	closed    bool
	listeners map[string]*gatewayListener
	conns     map[net.Conn]struct{}
	total     int
	wg        sync.WaitGroup
}

type gatewayListener struct {
	target TunnelTarget
	ln     net.Listener
	active atomic.Int64
}

// NewTunnelGateway prepares the socket directory (0700) and removes stale
// sockets left by a previous run. With the flag off it creates nothing.
func NewTunnelGateway(dir string, dialer TunnelDialer, logger *slog.Logger) (*TunnelGateway, error) {
	if !experimental.Enabled(experimental.AgentIngressTunnel) {
		return nil, ErrTunnelGatewayDisabled
	}
	if dialer == nil {
		return nil, errors.New("ingress: tunnel gateway needs a dialer")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := prepareSocketDir(dir); err != nil {
		return nil, err
	}
	if err := removeStaleSockets(dir); err != nil {
		return nil, err
	}
	return &TunnelGateway{
		dir: dir, dialer: dialer, logger: logger,
		maxTotal: tunnelGatewayMaxTotal, maxPerTarget: tunnelGatewayMaxPerTarget,
		openTimeout: tunnelGatewayOpenTimeout, idleTimeout: tunnelGatewayIdleTimeout,
		listeners: map[string]*gatewayListener{}, conns: map[net.Conn]struct{}{},
	}, nil
}

// prepareSocketDir makes the directory when missing, then refuses anything that
// is not a real directory (no symlink) owned by this user, then forces mode 0700.
func prepareSocketDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return fmt.Errorf("ingress: create tunnel socket dir: %w", err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("ingress: create tunnel socket dir: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("ingress: inspect tunnel socket dir: %w", err)
	}
	if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("ingress: tunnel socket dir must be a real directory, not a symlink")
	}
	if !ownedByCurrentUser(fi) {
		return errors.New("ingress: tunnel socket dir is not owned by the current user")
	}
	if fi.Mode().Perm() != 0o700 {
		// Only a real directory we own gets here, so tightening it cannot follow a link or touch another user's.
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // 0700 is the intended owner-only mode
			return fmt.Errorf("ingress: secure tunnel socket dir: %w", err)
		}
	}
	return nil
}

// removeStaleSockets deletes only socket files, so a mistaken dir cannot lose data.
func removeStaleSockets(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ingress: read tunnel socket dir: %w", err)
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("ingress: remove stale tunnel socket: %w", err)
		}
	}
	return nil
}

// TunnelUpstreamDial formats an absolute socket path as a Caddy upstream dial
// address: "unix/" plus the path, so "unix//abs/path" (Caddy's network address form).
func TunnelUpstreamDial(socketPath string) string { return "unix/" + socketPath }

// SocketPath is the deterministic socket path of a target.
func (g *TunnelGateway) SocketPath(t TunnelTarget) string {
	sum := sha256.Sum256([]byte(t.NodeID + "\x00" + t.Service + "\x00" + strconv.Itoa(t.ContainerPort)))
	return filepath.Join(g.dir, hex.EncodeToString(sum[:12])+".sock")
}

// Register starts serving a target (idempotent) and returns its socket path.
func (g *TunnelGateway) Register(t TunnelTarget) (string, error) {
	if t.NodeID == "" || t.Service == "" || t.ContainerPort <= 0 || t.ContainerPort > 65535 {
		return "", errors.New("ingress: invalid tunnel target")
	}
	path := g.SocketPath(t)
	if len(path) > tunnelGatewayMaxSocketPath {
		return "", errors.New("ingress: tunnel socket path too long, use a shorter data dir")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return "", errors.New("ingress: tunnel gateway closed")
	}
	if _, ok := g.listeners[path]; ok {
		return path, nil
	}
	_ = os.Remove(path)
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return "", fmt.Errorf("ingress: listen tunnel socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("ingress: secure tunnel socket: %w", err)
	}
	gl := &gatewayListener{target: t, ln: ln}
	g.listeners[path] = gl
	g.wg.Add(1)
	go g.accept(gl)
	return path, nil
}

// Sync keeps exactly the given targets served and closes the rest.
func (g *TunnelGateway) Sync(targets []TunnelTarget) error {
	want := map[string]bool{}
	var firstErr error
	for _, t := range targets {
		p, err := g.Register(t)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		want[p] = true
	}
	g.mu.Lock()
	for p, gl := range g.listeners {
		if !want[p] {
			_ = gl.ln.Close()
			_ = os.Remove(p)
			delete(g.listeners, p)
		}
	}
	g.mu.Unlock()
	return firstErr
}

// Close stops all listeners, cuts live connections and waits for goroutines.
func (g *TunnelGateway) Close() error {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		return nil
	}
	g.closed = true
	for p, gl := range g.listeners {
		_ = gl.ln.Close()
		_ = os.Remove(p)
	}
	g.listeners = map[string]*gatewayListener{}
	for c := range g.conns {
		_ = c.Close()
	}
	g.mu.Unlock()
	g.wg.Wait()
	return nil
}

func (g *TunnelGateway) accept(gl *gatewayListener) {
	defer g.wg.Done()
	for {
		c, err := gl.ln.Accept()
		if err != nil {
			return
		}
		if !g.admit(gl, c) {
			g.wg.Add(1)
			go func() {
				defer g.wg.Done()
				defer func() { _ = c.Close() }()
				writeGatewayError(c, 503, "tunnel capacity reached, retry shortly")
			}()
			continue
		}
		g.wg.Add(1)
		go func() {
			defer g.wg.Done()
			defer g.release(gl, c)
			g.serve(gl.target, c)
		}()
	}
}

func (g *TunnelGateway) admit(gl *gatewayListener, c net.Conn) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.total >= g.maxTotal || int(gl.active.Load()) >= g.maxPerTarget {
		return false
	}
	g.total++
	gl.active.Add(1)
	g.conns[c] = struct{}{}
	return true
}

func (g *TunnelGateway) release(gl *gatewayListener, c net.Conn) {
	g.mu.Lock()
	delete(g.conns, c)
	g.total--
	g.mu.Unlock()
	gl.active.Add(-1)
	_ = c.Close()
}

func (g *TunnelGateway) serve(t TunnelTarget, c net.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), g.openTimeout)
	ts, err := g.dialer.OpenTunnel(ctx, t.NodeID, t.Service, t.ContainerPort)
	cancel()
	if err != nil {
		code, msg := gatewayErrorFor(err)
		g.logger.Warn("ingress: tunnel open failed",
			slog.String("node", t.NodeID), slog.String("service", t.Service), slog.Int("status", code))
		writeGatewayError(c, code, msg)
		return
	}
	g.splice(c, ts)
}

func gatewayErrorFor(err error) (int, string) {
	switch {
	case errors.Is(err, ErrTunnelUnsupported):
		return 503, "the node agent does not support ingress tunnels, upgrade the agent"
	case errors.Is(err, ErrTunnelOffline):
		return 503, "the node is offline"
	case errors.Is(err, ErrTunnelTimeout):
		return 504, "the node did not answer in time"
	case errors.Is(err, context.DeadlineExceeded):
		return 502, "the node did not answer in time"
	default:
		return 502, "could not open a tunnel to the node"
	}
}

// writeGatewayError answers like an upstream so the proxy relays a clean
// status, then drains a little input so the peer does not see a reset first.
func writeGatewayError(c net.Conn, code int, msg string) {
	text := "Bad Gateway"
	switch code {
	case 503:
		text = "Service Unavailable"
	case 504:
		text = "Gateway Timeout"
	}
	_ = c.SetDeadline(time.Now().Add(tunnelGatewayErrDrainWindow))
	_, _ = fmt.Fprintf(c, "HTTP/1.1 %d %s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		code, text, len(msg), msg)
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	_, _ = io.CopyN(io.Discard, c, tunnelGatewayErrBodyDrain)
}

var gatewayBufs = sync.Pool{New: func() any { b := make([]byte, tunnelGatewayCopyBuffer); return &b }}

// splice pumps both directions with one bounded buffer each, propagates
// half-close, and tears both ends down on idle or error.
func (g *TunnelGateway) splice(c net.Conn, ts TunnelStream) {
	var once sync.Once
	closeBoth := func() { once.Do(func() { _ = c.Close(); _ = ts.Close() }) }
	defer closeBoth()
	idle := time.AfterFunc(g.idleTimeout, closeBoth)
	defer idle.Stop()

	var wg sync.WaitGroup
	pump := func(dst io.Writer, src io.Reader, closeWrite func() error) {
		defer wg.Done()
		bp := gatewayBufs.Get().(*[]byte)
		defer gatewayBufs.Put(bp)
		buf := *bp
		for {
			n, rerr := src.Read(buf)
			if n > 0 {
				idle.Reset(g.idleTimeout)
				if _, werr := dst.Write(buf[:n]); werr != nil {
					closeBoth()
					return
				}
			}
			if rerr != nil {
				if errors.Is(rerr, io.EOF) {
					_ = closeWrite()
				} else {
					closeBoth()
				}
				return
			}
		}
	}
	localCW := func() error {
		if cw, ok := c.(interface{ CloseWrite() error }); ok {
			return cw.CloseWrite()
		}
		return c.Close()
	}
	wg.Add(2)
	go pump(ts, c, ts.CloseWrite)
	go pump(c, ts, localCW)
	wg.Wait()
}
