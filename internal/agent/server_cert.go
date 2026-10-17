package agent

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Renew implements agentpb.AgentServiceServer. The CSR is signed before the
// store is updated: if the update fails, the new certificate was never
// recorded and so is never trusted.
func (s *Server) Renew(ctx context.Context, req *agentpb.RenewRequest) (*agentpb.RenewResponse, error) {
	if len(req.GetCsrDer()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "csr_der is required")
	}
	node, peerCert, err := s.authenticatePeer(ctx)
	if err != nil {
		return nil, err
	}
	if node.CertKeyOrigin == store.CertKeyOriginReenrollRequired {
		s.recordRenewFailed(ctx, node.ID, store.SecurityReasonReenrollRequired)
		return nil, status.Errorf(codes.FailedPrecondition, "node %q still uses a key the control plane generated: re-enroll the node", node.ID)
	}
	issued, err := s.signCSR(node.ID, req.GetCsrDer())
	if err != nil {
		s.recordRenewFailed(ctx, node.ID, store.SecurityReasonSignFailed)
		return nil, err
	}
	presented := CertFingerprint(peerCert.Raw)
	err = s.store.RotateNodeCert(ctx, node.ID, presented, store.NodeCert{
		Fingerprint: issued.Fingerprint, Serial: issued.Serial, NotAfter: issued.NotAfter, KeyOrigin: store.CertKeyOriginAgent,
	}, s.renewGrace, s.now())
	switch {
	case err == nil:
	case errors.Is(err, store.ErrNodeCertRevoked):
		s.recordRenewFailed(ctx, node.ID, store.SecurityReasonRevoked)
		return nil, status.Errorf(codes.PermissionDenied, "certificate for node %q was revoked: re-enroll the node", node.ID)
	case errors.Is(err, store.ErrNodeCertNotAccepted), errors.Is(err, store.ErrNodeNotFound):
		s.recordRenewFailed(ctx, node.ID, store.SecurityReasonFingerprintMismatch)
		return nil, status.Errorf(codes.Unauthenticated, "certificate no longer accepted for node %q", node.ID)
	case errors.Is(err, store.ErrNodeCertConflict):
		s.recordRenewFailed(ctx, node.ID, store.SecurityReasonRenewConflict)
		return nil, status.Error(codes.Aborted, "a concurrent renewal won: retry")
	default:
		s.logger.Error("agent: renew: record cert failed", slog.String("node_id", node.ID), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	s.recordCertIssued(ctx, node.ID, "renew", issued)
	s.logger.Info("agent: node certificate renewed", slog.String("node_id", node.ID), slog.Time("not_after", issued.NotAfter))
	return &agentpb.RenewResponse{
		ClientCertPem: issued.PEM,
		CaCertPem:     s.ca.CertPEM(),
		NotAfter:      timestamppb.New(issued.NotAfter),
	}, nil
}

// Reenroll implements agentpb.AgentServiceServer: a token bound to an
// existing node buys that node a new certificate. Minting the token was the
// operator's authorization, so a revocation is cleared.
func (s *Server) Reenroll(ctx context.Context, req *agentpb.ReenrollRequest) (*agentpb.ReenrollResponse, error) {
	if req.GetReenrollToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "reenroll_token is required")
	}
	if len(req.GetCsrDer()) == 0 {
		s.recordUpgradeRequired(ctx, store.SecurityKindReenroll, req.GetNodeId())
		return nil, errAgentUpgradeRequired
	}
	tok, err := s.redeemToken(ctx, req.GetReenrollToken(), store.NodeJoinTokenPurposeReenroll)
	if err != nil {
		s.recordTokenRefusal(ctx, store.SecurityKindReenroll, "", err)
		return nil, err
	}
	nodeID := tok.NodeID
	if req.GetNodeId() != "" && req.GetNodeId() != nodeID {
		return nil, status.Error(codes.PermissionDenied, "re-enrollment token was issued for a different node")
	}
	if _, err := s.store.GetNode(ctx, nodeID); err != nil {
		if errors.Is(err, store.ErrNodeNotFound) {
			return nil, status.Errorf(codes.NotFound, "node %q no longer exists", nodeID)
		}
		s.logger.Error("agent: reenroll: look up node failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	issued, err := s.signCSR(nodeID, req.GetCsrDer())
	if err != nil {
		return nil, err
	}
	if err := s.store.ReenrollNodeCert(ctx, nodeID, store.NodeCert{
		Fingerprint: issued.Fingerprint, Serial: issued.Serial, NotAfter: issued.NotAfter, KeyOrigin: store.CertKeyOriginAgent,
	}, s.now()); err != nil {
		s.logger.Error("agent: reenroll: record cert failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	s.recordAgentInfo(ctx, nodeID, req.GetAgent())
	s.recordEnrolled(ctx, store.SecurityKindReenroll, nodeID, tok.ID, req.GetCaPinState(), issued)
	s.logger.Info("agent: node re-enrolled", slog.String("node_id", nodeID), slog.Time("not_after", issued.NotAfter))
	return &agentpb.ReenrollResponse{
		NodeId:        nodeID,
		ClientCertPem: issued.PEM,
		CaCertPem:     s.ca.CertPEM(),
		NotAfter:      timestamppb.New(issued.NotAfter),
	}, nil
}

// CheckIdentity implements agentpb.AgentServiceServer.
func (s *Server) CheckIdentity(ctx context.Context, _ *agentpb.CheckIdentityRequest) (*agentpb.CheckIdentityResponse, error) {
	node, peerCert, err := s.authenticatePeer(ctx)
	if err != nil {
		return nil, err
	}
	return &agentpb.CheckIdentityResponse{
		NodeId:  node.ID,
		Current: CertFingerprint(peerCert.Raw) == node.CertFingerprint,
	}, nil
}
