package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

// TunnelOpener is what the ingress gateway needs: open a tunnel to a node's own service port.
// deadline is the absolute end of the tunnel's life (zero means idle timeout only).
type TunnelOpener interface {
	OpenTunnel(ctx context.Context, nodeID, service string, containerPort uint16, deadline time.Time) (*TunnelConn, error)
}

var (
	_ TunnelOpener = (*GRPCTransport)(nil)
	_ TunnelOpener = (*Server)(nil)
)

func (t *GRPCTransport) attachTunnels(h *tunnelHub, nodeID string) {
	t.tunnels, t.nodeID = h, nodeID
}

// TunnelsViaAgent reports whether this node's agent advertises ingress tunnels.
func (t *GRPCTransport) TunnelsViaAgent() bool { return t.caps.has(CapIngressTunnel) }

// OpenTunnel asks the agent to open a tunnel to its own service port and waits, within the
// open timeout, for the agent's hello. It never sends a host or address; the agent resolves the port.
func (t *GRPCTransport) OpenTunnel(ctx context.Context, nodeID, service string, containerPort uint16, deadline time.Time) (*TunnelConn, error) {
	if !experimental.Enabled(experimental.AgentIngressTunnel) {
		return nil, ErrTunnelUnsupported
	}
	if !t.caps.has(CapIngressTunnel) {
		return nil, fmt.Errorf("%w: %w: ingress tunnel needs %s", ErrTunnelUnsupported, ErrAgentLacksCapability, CapIngressTunnel)
	}
	h := t.tunnels
	if h == nil {
		return nil, ErrTunnelUnsupported
	}
	if nodeID != t.nodeID {
		return nil, ErrNodeOffline
	}
	if service == "" || containerPort == 0 {
		return nil, fmt.Errorf("%w: service and port are required", ErrTunnelRefused)
	}
	c, err := t.openTunnel(ctx, h, service, containerPort, deadline)
	switch {
	case err == nil:
		h.result("ok")
	case errors.Is(err, ErrTunnelLimit):
		h.result("limit")
	case errors.Is(err, ErrTunnelTimeout):
		h.result("timeout")
	case errors.Is(err, ErrNodeOffline):
		h.result("offline")
	default:
		h.result("refused")
	}
	if err != nil {
		h.logger.Debug("agent: tunnel open failed", slog.String("node_id", nodeID), slog.String("service", service), slog.String("error", err.Error()))
	}
	return c, err
}

func (t *GRPCTransport) openTunnel(ctx context.Context, h *tunnelHub, service string, port uint16, deadline time.Time) (*TunnelConn, error) {
	if err := h.reserve(t.nodeID, service); err != nil {
		return nil, err
	}
	id, err := newTunnelID()
	if err != nil {
		h.release(t.nodeID, service)
		return nil, err
	}
	key := string(id)
	p := &pendingTunnel{nodeID: t.nodeID, service: service, owner: t.mux, ready: make(chan *TunnelConn, 1)}
	h.mu.Lock()
	h.pending[key] = p
	h.mu.Unlock()

	// fail drops the pending entry; if the Tunnel RPC already took it, the conn owns the slot and is closed instead.
	fail := func(err error) (*TunnelConn, error) {
		h.mu.Lock()
		_, still := h.pending[key]
		delete(h.pending, key)
		h.mu.Unlock()
		if still {
			h.release(t.nodeID, service)
			return nil, err
		}
		go func() {
			if c := <-p.ready; c != nil {
				c.closeWith(err)
			}
		}()
		return nil, err
	}

	limits := h.lim()
	// The agent treats this as the tunnel's whole lifetime; the open timeout stays local to the handshake.
	life := TunnelMaxDeadline
	if !deadline.IsZero() {
		life = time.Until(deadline)
		if life <= 0 {
			return fail(ErrTunnelTimeout)
		}
	}
	hctx, cancel := context.WithTimeout(ctx, limits.OpenTimeout)
	defer cancel()
	resp, err := t.mux.Call(hctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_TunnelOpen{TunnelOpen: &agentpb.TunnelOpenRequest{
		TunnelId: id, Service: service, ContainerPort: uint32(port), DeadlineNs: int64(life),
	}}})
	if err != nil {
		return fail(mapOpenErr(hctx, ctx, err))
	}
	if reason := resp.GetTunnelOpen().GetFailureReason(); reason != "" {
		return fail(&TunnelRefusedError{Reason: cleanReason(reason)})
	}
	select {
	case c := <-p.ready:
		c.armTimers(limits.IdleTimeout, deadline)
		return c, nil
	case <-hctx.Done():
		return fail(mapOpenErr(hctx, ctx, hctx.Err()))
	case <-t.mux.closed:
		return fail(ErrNodeOffline)
	}
}

// mapOpenErr turns a Call failure into a typed error; the agent's error text is bounded before it is kept.
func mapOpenErr(hctx, parent context.Context, err error) error {
	switch {
	case parent.Err() != nil:
		return parent.Err()
	case errors.Is(err, context.DeadlineExceeded), hctx.Err() != nil:
		return ErrTunnelTimeout
	case errors.Is(err, ErrSessionClosed):
		return fmt.Errorf("%w: %w", ErrNodeOffline, err)
	default:
		return &TunnelRefusedError{Reason: cleanReason(err.Error())}
	}
}

// OpenTunnel finds the node's live session and opens the tunnel through it.
func (s *Server) OpenTunnel(ctx context.Context, nodeID, service string, containerPort uint16, deadline time.Time) (*TunnelConn, error) {
	if !experimental.Enabled(experimental.AgentIngressTunnel) {
		return nil, ErrTunnelUnsupported
	}
	tp, err := s.registry.Get(nodeID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNodeOffline, err)
	}
	g, ok := tp.(*GRPCTransport)
	if !ok {
		return nil, ErrTunnelUnsupported
	}
	return g.OpenTunnel(ctx, nodeID, service, containerPort, deadline)
}

// NodeTunnels reports whether the node has a live session whose agent advertises ingress tunnels.
func (s *Server) NodeTunnels(nodeID string) bool {
	tp, err := s.registry.Get(nodeID)
	if err != nil {
		return false
	}
	g, ok := tp.(*GRPCTransport)
	return ok && g.TunnelsViaAgent()
}

// NodeConnected reports whether the node has a live agent session.
func (s *Server) NodeConnected(nodeID string) bool {
	_, err := s.registry.Get(nodeID)
	return err == nil
}

// NodeCapabilities lists what the node's live session advertised, sorted; nil when offline.
func (s *Server) NodeCapabilities(nodeID string) []string {
	tp, err := s.registry.Get(nodeID)
	if err != nil {
		return nil
	}
	g, ok := tp.(*GRPCTransport)
	if !ok {
		return nil
	}
	return g.caps.list()
}
