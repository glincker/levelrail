package agent

// Control plane side of ingress tunnels: the pending-open table, the Tunnel
// RPC handler and the connection object the ingress gateway wraps as a net.Conn.

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

// Typed open and stream errors the gateway maps to HTTP statuses.
var (
	ErrNodeOffline   = errors.New("agent: node is offline")
	ErrTunnelRefused = errors.New("agent: tunnel refused by the agent")
	ErrTunnelLimit   = errors.New("agent: tunnel limit reached")
	ErrTunnelTimeout = errors.New("agent: tunnel open timed out")
	ErrTunnelIdle    = errors.New("agent: tunnel idle timeout")
	ErrTunnelExpired = errors.New("agent: tunnel lifetime exceeded")
	ErrTunnelClosed  = errors.New("agent: tunnel closed")
	ErrTunnelReset   = errors.New("agent: tunnel reset by the agent")
)

// TunnelRefusedError carries the agent's short refusal reason.
type TunnelRefusedError struct{ Reason string }

// Error implements error.
func (e *TunnelRefusedError) Error() string { return "agent: tunnel refused: " + e.Reason }

// Is matches ErrTunnelRefused.
func (e *TunnelRefusedError) Is(target error) bool { return target == ErrTunnelRefused }

// TunnelResetError is returned by Read when the agent ends the tunnel with a reason.
type TunnelResetError struct{ Reason string }

// Error implements error.
func (e *TunnelResetError) Error() string { return "agent: tunnel reset: " + e.Reason }

// Is matches ErrTunnelReset.
func (e *TunnelResetError) Is(target error) bool { return target == ErrTunnelReset }

// Env names seeding the tunnel limits; runtime settings may override them live.
const (
	EnvTunnelMaxPerNode    = "APP_INGRESS_TUNNEL_MAX_STREAMS_PER_NODE"
	EnvTunnelMaxPerService = "APP_INGRESS_TUNNEL_MAX_STREAMS_PER_SERVICE"
	EnvTunnelOpenTimeout   = "APP_INGRESS_TUNNEL_OPEN_TIMEOUT"
	EnvTunnelIdleTimeout   = "APP_INGRESS_TUNNEL_IDLE_TIMEOUT"
	// EnvTunnelMaxPendingHellos caps Tunnel RPCs per node that still wait for their hello.
	EnvTunnelMaxPendingHellos = "APP_INGRESS_TUNNEL_MAX_PENDING_HELLOS"
)

// TunnelLimits bounds tunnels per node and in time. Zero fields fall back to the defaults.
type TunnelLimits struct {
	MaxPerNode, MaxPerService int
	// MaxPendingHellos bounds unauthenticated-by-hello streams a node may hold open.
	MaxPendingHellos         int
	OpenTimeout, IdleTimeout time.Duration
}

// DefaultTunnelLimits are the design defaults used when nothing is configured.
func DefaultTunnelLimits() TunnelLimits {
	return TunnelLimits{MaxPerNode: 256, MaxPerService: 128, MaxPendingHellos: 64, OpenTimeout: 5 * time.Second, IdleTimeout: 10 * time.Minute}
}

func (l TunnelLimits) withDefaults() TunnelLimits {
	d := DefaultTunnelLimits()
	if l.MaxPerNode <= 0 {
		l.MaxPerNode = d.MaxPerNode
	}
	if l.MaxPerService <= 0 {
		l.MaxPerService = d.MaxPerService
	}
	if l.MaxPendingHellos <= 0 {
		l.MaxPendingHellos = d.MaxPendingHellos
	}
	if l.OpenTimeout <= 0 {
		l.OpenTimeout = d.OpenTimeout
	}
	if l.IdleTimeout <= 0 {
		l.IdleTimeout = d.IdleTimeout
	}
	return l
}

// TunnelLimitsFromEnv seeds the limits; a bad value is an error so a typo is not silently ignored.
func TunnelLimitsFromEnv(lookup func(string) (string, bool)) (TunnelLimits, error) {
	l := DefaultTunnelLimits()
	get := func(k string) string { v, _ := lookup(k); return strings.TrimSpace(v) }
	for _, n := range []struct {
		key string
		dst *int
	}{{EnvTunnelMaxPerNode, &l.MaxPerNode}, {EnvTunnelMaxPerService, &l.MaxPerService}, {EnvTunnelMaxPendingHellos, &l.MaxPendingHellos}} {
		if v := get(n.key); v != "" {
			p, err := strconv.Atoi(v)
			if err != nil || p <= 0 {
				return TunnelLimits{}, fmt.Errorf("%s must be a positive integer", n.key)
			}
			*n.dst = p
		}
	}
	for _, d := range []struct {
		key string
		dst *time.Duration
	}{{EnvTunnelOpenTimeout, &l.OpenTimeout}, {EnvTunnelIdleTimeout, &l.IdleTimeout}} {
		if v := get(d.key); v != "" {
			p, err := time.ParseDuration(v)
			if err != nil || p <= 0 {
				return TunnelLimits{}, fmt.Errorf("%s must be a positive duration", d.key)
			}
			*d.dst = p
		}
	}
	return l, nil
}

// TunnelStats is a counter snapshot; it never holds payload or headers.
type TunnelStats struct {
	OpenResults     map[string]uint64
	ActiveByNode    map[string]int
	BytesToAgent    uint64
	BytesFromAgent  uint64
	ActiveTunnels   int
	OpenedTunnels   uint64
	RefusedOrFailed uint64
}

type pendingTunnel struct {
	nodeID  string
	service string
	owner   *mux
	ready   chan *TunnelConn
}

// tunnelHub owns the pending opens and live tunnels of one control plane.
type tunnelHub struct {
	logger *slog.Logger
	limits atomic.Pointer[TunnelLimits]

	mu        sync.Mutex
	pending   map[string]*pendingTunnel
	perNode   map[string]int
	perSvc    map[string]int
	hellos    map[string]int
	conns     map[*TunnelConn]struct{}
	results   map[string]uint64
	toAgent   atomic.Uint64
	fromAgent atomic.Uint64
}

func newTunnelHub(logger *slog.Logger) *tunnelHub {
	h := &tunnelHub{
		logger: logger, pending: map[string]*pendingTunnel{}, perNode: map[string]int{},
		perSvc: map[string]int{}, hellos: map[string]int{}, conns: map[*TunnelConn]struct{}{}, results: map[string]uint64{},
	}
	l := DefaultTunnelLimits()
	h.limits.Store(&l)
	return h
}

func (h *tunnelHub) setLimits(l TunnelLimits) {
	l = l.withDefaults()
	h.limits.Store(&l)
}

func (h *tunnelHub) lim() TunnelLimits { return *h.limits.Load() }

func (h *tunnelHub) result(r string) {
	h.mu.Lock()
	h.results[r]++
	h.mu.Unlock()
}

func svcKey(node, service string) string { return node + "\x00" + service }

// reserve takes one per-node and per-service slot, or reports ErrTunnelLimit.
func (h *tunnelHub) reserve(node, service string) error {
	l := h.lim()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perNode[node] >= l.MaxPerNode || h.perSvc[svcKey(node, service)] >= l.MaxPerService {
		return ErrTunnelLimit
	}
	h.perNode[node]++
	h.perSvc[svcKey(node, service)]++
	return nil
}

func (h *tunnelHub) release(node, service string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.perNode[node]--; h.perNode[node] <= 0 {
		delete(h.perNode, node)
	}
	k := svcKey(node, service)
	if h.perSvc[k]--; h.perSvc[k] <= 0 {
		delete(h.perSvc, k)
	}
}

func (h *tunnelHub) stats() TunnelStats {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := TunnelStats{
		OpenResults: map[string]uint64{}, ActiveByNode: map[string]int{},
		BytesToAgent: h.toAgent.Load(), BytesFromAgent: h.fromAgent.Load(), ActiveTunnels: len(h.conns),
	}
	for k, v := range h.results {
		s.OpenResults[k] = v
		if k == "ok" {
			s.OpenedTunnels = v
		} else {
			s.RefusedOrFailed += v
		}
	}
	for c := range h.conns {
		s.ActiveByNode[c.node]++
	}
	return s
}

// closeOwner tears down every tunnel opened over a session that just ended.
func (h *tunnelHub) closeOwner(m *mux) {
	h.mu.Lock()
	var victims []*TunnelConn
	for c := range h.conns {
		if c.owner == m {
			victims = append(victims, c)
		}
	}
	h.mu.Unlock()
	for _, c := range victims {
		c.closeWith(ErrNodeOffline)
	}
}

func newTunnelID() ([]byte, error) {
	id := make([]byte, TunnelIDLen)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("agent: tunnel id: %w", err)
	}
	return id, nil
}

// tunnelStream is the narrow surface of the Tunnel RPC stream, so tests can fake it.
type tunnelStream interface {
	Send(*agentpb.TunnelFrame) error
	Recv() (*agentpb.TunnelFrame, error)
}

// TunnelConn is one open tunnel. The gateway wraps it as a net.Conn.
type TunnelConn struct {
	hub     *tunnelHub
	owner   *mux
	node    string
	service string
	stream  tunnelStream

	// pr is read by the gateway; closing only pw keeps typed errors visible to Read.
	pr *io.PipeReader
	pw *io.PipeWriter

	sendMu     sync.Mutex
	halfClosed bool

	done      chan struct{}
	closeOnce sync.Once
	closeErr  atomic.Pointer[error]
	reason    atomic.Pointer[string]
	lastNanos atomic.Int64
	timerMu   sync.Mutex
	idle      *time.Timer
	life      *time.Timer
}

func (h *tunnelHub) newConn(node, service string, owner *mux, st tunnelStream) *TunnelConn {
	pr, pw := io.Pipe()
	c := &TunnelConn{hub: h, owner: owner, node: node, service: service, stream: st, pr: pr, pw: pw, done: make(chan struct{})}
	c.touch()
	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (c *TunnelConn) touch() { c.lastNanos.Store(time.Now().UnixNano()) }

// armTimers starts the idle watch and, when lifetime is set, the absolute deadline.
func (c *TunnelConn) armTimers(idle time.Duration, lifetime time.Time) {
	c.timerMu.Lock()
	defer c.timerMu.Unlock()
	var check func()
	check = func() {
		sinceLast := time.Since(time.Unix(0, c.lastNanos.Load()))
		if sinceLast >= idle {
			c.closeWith(ErrTunnelIdle)
			return
		}
		c.timerMu.Lock()
		defer c.timerMu.Unlock()
		select {
		case <-c.done:
		default:
			c.idle = time.AfterFunc(idle-sinceLast, check)
		}
	}
	c.idle = time.AfterFunc(idle, check)
	if !lifetime.IsZero() {
		c.life = time.AfterFunc(time.Until(lifetime), func() { c.closeWith(ErrTunnelExpired) })
	}
}

// Read returns the agent's bytes; io.EOF follows a half close, a TunnelResetError a reset.
func (c *TunnelConn) Read(p []byte) (int, error) {
	n, err := c.pr.Read(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

// Write sends p in frames of at most TunnelMaxFrameData; gRPC flow control makes it block when the agent is slow.
func (c *TunnelConn) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > TunnelMaxFrameData {
			chunk = chunk[:TunnelMaxFrameData]
		}
		if err := c.send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Data{Data: chunk}}, false); err != nil {
			return written, err
		}
		c.hub.toAgent.Add(uint64(len(chunk)))
		c.touch()
		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}

func (c *TunnelConn) send(f *agentpb.TunnelFrame, half bool) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	select {
	case <-c.done:
		return c.err()
	default:
	}
	if c.halfClosed {
		return io.ErrClosedPipe
	}
	if err := c.stream.Send(f); err != nil {
		return fmt.Errorf("agent: tunnel send: %w", err)
	}
	c.halfClosed = c.halfClosed || half
	return nil
}

// CloseWrite tells the agent no more bytes follow; reads stay open.
func (c *TunnelConn) CloseWrite() error {
	return c.send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_HalfClose{HalfClose: &agentpb.Empty{}}}, true)
}

// Close ends the tunnel and releases its slot; safe to call more than once.
func (c *TunnelConn) Close() error {
	c.closeWith(ErrTunnelClosed)
	return nil
}

// ResetReason is the agent's reset reason, empty when it did not reset.
func (c *TunnelConn) ResetReason() string {
	if r := c.reason.Load(); r != nil {
		return *r
	}
	return ""
}

// Done closes when the tunnel has ended for any reason.
func (c *TunnelConn) Done() <-chan struct{} { return c.done }

func (c *TunnelConn) err() error {
	if e := c.closeErr.Load(); e != nil {
		return *e
	}
	return ErrTunnelClosed
}

func (c *TunnelConn) closeWith(err error) {
	c.closeOnce.Do(func() {
		c.closeErr.Store(&err)
		close(c.done)
		c.pw.CloseWithError(err)
		c.timerMu.Lock()
		if c.idle != nil {
			c.idle.Stop()
		}
		if c.life != nil {
			c.life.Stop()
		}
		c.timerMu.Unlock()
		c.hub.mu.Lock()
		delete(c.hub.conns, c)
		c.hub.mu.Unlock()
		c.hub.release(c.node, c.service)
		c.hub.logger.Debug("agent: tunnel closed", slog.String("node_id", c.node), slog.String("service", c.service), slog.String("cause", err.Error()))
	})
}

// pump copies the agent's frames into the read pipe until the tunnel ends.
func (c *TunnelConn) pump() {
	for {
		f, err := c.stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.closeWith(&TunnelResetError{Reason: "stream ended"})
				return
			}
			c.closeWith(fmt.Errorf("%w: %w", ErrNodeOffline, err))
			return
		}
		switch fr := f.GetFrame().(type) {
		case *agentpb.TunnelFrame_Data:
			if len(fr.Data) > TunnelMaxFrameData {
				c.closeWith(&TunnelResetError{Reason: "frame too large"})
				return
			}
			c.hub.fromAgent.Add(uint64(len(fr.Data)))
			if _, werr := c.pw.Write(fr.Data); werr != nil {
				return
			}
			c.touch()
		case *agentpb.TunnelFrame_HalfClose:
			_ = c.pw.Close()
		case *agentpb.TunnelFrame_Reset_:
			reason := cleanReason(fr.Reset_.GetReason())
			c.reason.Store(&reason)
			c.closeWith(&TunnelResetError{Reason: reason})
			return
		default:
			c.closeWith(&TunnelResetError{Reason: "unexpected frame"})
			return
		}
	}
}

// cleanReason bounds an agent-supplied string and drops control characters before it is logged or shown.
func cleanReason(s string) string {
	if len(s) > TunnelMaxResetReason {
		s = s[:TunnelMaxResetReason]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// serveTunnel binds one Tunnel RPC to its pending open and carries it until it ends.
// nodeID comes from the peer certificate, never from a frame.
func (h *tunnelHub) serveTunnel(ctx context.Context, nodeID string, stream grpc.BidiStreamingServer[agentpb.TunnelFrame, agentpb.TunnelFrame]) error {
	type first struct {
		f   *agentpb.TunnelFrame
		err error
	}
	h.mu.Lock()
	if h.hellos[nodeID] >= h.lim().MaxPendingHellos {
		h.mu.Unlock()
		return status.Error(codes.ResourceExhausted, "too many tunnels awaiting hello")
	}
	h.hellos[nodeID]++
	h.mu.Unlock()
	var helloDone sync.Once
	endWait := func() {
		helloDone.Do(func() {
			h.mu.Lock()
			if h.hellos[nodeID]--; h.hellos[nodeID] <= 0 {
				delete(h.hellos, nodeID)
			}
			h.mu.Unlock()
		})
	}
	defer endWait()
	got := make(chan first, 1)
	go func() {
		f, err := stream.Recv()
		got <- first{f, err}
	}()
	timer := time.NewTimer(h.lim().OpenTimeout)
	defer timer.Stop()
	var hello *agentpb.TunnelHello
	select {
	case r := <-got:
		if r.err != nil {
			return status.Error(codes.Canceled, "tunnel ended before hello")
		}
		hello = r.f.GetHello()
	case <-timer.C:
		return status.Error(codes.DeadlineExceeded, "tunnel hello timed out")
	case <-ctx.Done():
		return ctx.Err()
	}
	endWait()
	if hello == nil || len(hello.GetTunnelId()) != TunnelIDLen {
		return status.Error(codes.InvalidArgument, "tunnel hello required")
	}
	id := string(hello.GetTunnelId())
	h.mu.Lock()
	p, ok := h.pending[id]
	switch {
	case !ok:
		h.mu.Unlock()
		return status.Error(codes.NotFound, "unknown tunnel")
	case p.nodeID != nodeID:
		h.mu.Unlock()
		h.logger.Warn("agent: tunnel hello from a different node", slog.String("node_id", nodeID))
		return status.Error(codes.PermissionDenied, "tunnel refused")
	}
	delete(h.pending, id)
	h.mu.Unlock()

	c := h.newConn(nodeID, p.service, p.owner, stream)
	p.ready <- c
	// The hello reader has finished, so pump is the only Recv caller from here.
	go c.pump()
	select {
	case <-c.done:
	case <-ctx.Done():
		c.closeWith(ErrNodeOffline)
	}
	return nil
}

// Tunnel implements the data RPC: it authenticates the peer, then binds the stream to its pending open.
func (s *Server) Tunnel(stream grpc.BidiStreamingServer[agentpb.TunnelFrame, agentpb.TunnelFrame]) error {
	if !experimental.Enabled(experimental.AgentIngressTunnel) {
		return status.Error(codes.Unimplemented, ErrTunnelUnsupported.Error())
	}
	node, _, err := s.authenticatePeer(stream.Context())
	if err != nil {
		return err
	}
	return s.tunnels.serveTunnel(stream.Context(), node.ID, stream)
}

// SetTunnelLimits applies new tunnel limits live; open tunnels keep running.
func (s *Server) SetTunnelLimits(l TunnelLimits) { s.tunnels.setLimits(l) }

// TunnelStats returns counters for metrics and the node page.
func (s *Server) TunnelStats() TunnelStats { return s.tunnels.stats() }
