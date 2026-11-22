package agent

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/store"
)

// UpdateDecider answers which agent version a node should run.
type UpdateDecider func(ctx context.Context, node *store.Node) nodeinsight.UpdateDecision

// SetUpdateDecider wires the decision source; safe to call after the server starts.
func (s *Server) SetUpdateDecider(d UpdateDecider) { s.updateDecider.Store(&d) }

const updateCacheTTL = 30 * time.Second

type cachedUpdate struct {
	at time.Time
	d  nodeinsight.UpdateDecision
}

// cachedUpdate answers repeat checks from one node from memory for a short
// time, so a looping agent cannot turn this call into store load.
func (s *Server) cachedUpdate(ctx context.Context, node *store.Node, decide UpdateDecider) nodeinsight.UpdateDecision {
	now := s.now()
	s.updateMu.Lock()
	if c, ok := s.updateCache[node.ID]; ok && now.Sub(c.at) < updateCacheTTL {
		s.updateMu.Unlock()
		return c.d
	}
	s.updateMu.Unlock()
	d := decide(ctx, node)
	s.updateMu.Lock()
	if s.updateCache == nil {
		s.updateCache = map[string]cachedUpdate{}
	}
	s.updateCache[node.ID] = cachedUpdate{at: now, d: d}
	s.updateMu.Unlock()
	return d
}

// UpdateCheck implements agentpb.AgentServiceServer. The node comes only from
// the verified client certificate, so there is nothing to spoof or cross-read.
func (s *Server) UpdateCheck(ctx context.Context, _ *agentpb.UpdateCheckRequest) (*agentpb.UpdateCheckResponse, error) {
	node, _, err := s.authenticatePeer(ctx)
	if err != nil {
		return nil, err
	}
	p := s.updateDecider.Load()
	if p == nil || *p == nil {
		return nil, status.Error(codes.Unavailable, "update checks are not ready")
	}
	d := s.cachedUpdate(ctx, node, *p)
	return &agentpb.UpdateCheckResponse{
		Action: d.Action, Channel: d.Channel, TargetVersion: d.Target, Reason: d.Reason, AutoApply: d.AutoApply,
	}, nil
}

// UpdateCheckAt returns the node's update decision, dialing addr with the
// node's own mTLS identity. No token is involved.
func UpdateCheckAt(ctx context.Context, addr string, id *Identity) (nodeinsight.UpdateDecision, error) {
	conn, err := dialWithIdentity(addr, id)
	if err != nil {
		return nodeinsight.UpdateDecision{}, err
	}
	defer func() { _ = conn.Close() }()
	resp, err := agentpb.NewAgentServiceClient(conn).UpdateCheck(ctx, &agentpb.UpdateCheckRequest{})
	if err != nil {
		return nodeinsight.UpdateDecision{}, err
	}
	return nodeinsight.UpdateDecision{
		Action: resp.GetAction(), Channel: resp.GetChannel(), Target: resp.GetTargetVersion(),
		Reason: resp.GetReason(), AutoApply: resp.GetAutoApply(),
	}, nil
}
